package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/api/internal/scan"
	"github.com/ymd38/goodast/jobs"
)

// ScanHandler はスキャン開始（enqueue）の HTTP ハンドラ。実行は worker が非同期に行う（ADR-0001）。
type ScanHandler struct {
	svc    *scan.Service
	logger *slog.Logger
}

// ScanHandlerDeps は ScanHandler の依存（dig struct-based injection）。
type ScanHandlerDeps struct {
	dig.In
	Service *scan.Service
	Logger  *slog.Logger
}

// NewScanHandler は ScanHandler を生成する。
func NewScanHandler(d ScanHandlerDeps) *ScanHandler {
	return &ScanHandler{svc: d.Service, logger: d.Logger}
}

// RegisterRoutes は scan 関連のルートを登録する。
func (h *ScanHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/scans", h.start)
}

type startScanRequest struct {
	SiteID string `json:"site_id" binding:"required"`
	Preset string `json:"preset"` // 省略可。空なら standard（jobs.ParsePreset）。
}

type startScanResponse struct {
	ScanID string `json:"scan_id"`
	Status string `json:"status"`
	Preset string `json:"preset"`
}

// start はスキャンを受け付ける。enqueue のみで実行完了を待たないため 202 Accepted を返す。
//
// @Summary      スキャンを開始
// @Description  site_id のスキャンを enqueue する（worker が非同期実行）。所有確認前は 403。202 で scan_id/status=queued を返す。preset（light/standard/deep・省略時 standard）を任意で受け付ける。
// @Tags         scans
// @Accept       json
// @Produce      json
// @Param        request  body      startScanRequest   true  "スキャン対象サイト"
// @Success      202      {object}  startScanResponse
// @Failure      400      {object}  handler.ErrorResponse
// @Failure      403      {object}  handler.ErrorResponse
// @Failure      404      {object}  handler.ErrorResponse
// @Failure      500      {object}  handler.ErrorResponse
// @Router       /scans [post]
func (h *ScanHandler) start(c *gin.Context) {
	var req startScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	siteID, err := uuid.Parse(req.SiteID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site_id"})
		return
	}

	preset, err := jobs.ParsePreset(req.Preset)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preset"})
		return
	}

	scanID, err := h.svc.EnqueueScan(c.Request.Context(), siteID, preset)
	if err != nil {
		h.writeScanError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, startScanResponse{ScanID: scanID.String(), Status: "queued", Preset: preset.String()})
}

// writeScanError はサービスのドメインエラーを HTTP ステータスへ対応づける。
func (h *ScanHandler) writeScanError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, scan.ErrSiteNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, scan.ErrOwnershipNotVerified):
		// 所有確認前のスキャンは安全ガードレールとして禁止（ADR-0004）。
		// /sites/:id/verify を通せば解消できるため、恒久拒否ではなく前提条件不足の 403。
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, scan.ErrScanInProgress):
		// 同一サイトで実行中のスキャンが既にある。完了後に再実行できるため 409 Conflict。
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, jobs.ErrInvalidPreset):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		h.logger.Error("enqueue scan failed", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
