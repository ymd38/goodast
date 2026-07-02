package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/api/internal/scan"
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
}

type startScanResponse struct {
	ScanID string `json:"scan_id"`
	Status string `json:"status"`
}

// start はスキャンを受け付ける。enqueue のみで実行完了を待たないため 202 Accepted を返す。
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

	scanID, err := h.svc.EnqueueScan(c.Request.Context(), siteID)
	if err != nil {
		h.writeScanError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, startScanResponse{ScanID: scanID.String(), Status: "queued"})
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
	default:
		h.logger.Error("enqueue scan failed", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
