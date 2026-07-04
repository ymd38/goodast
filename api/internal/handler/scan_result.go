package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/api/internal/report"
)

// ScanResultHandler は 1 スキャンの結果（状態＋明細）の読み取り HTTP ハンドラ。
// 状態（サマリ）と明細（findings）をエンドポイントで分離する。
type ScanResultHandler struct {
	svc    *report.Service
	logger *slog.Logger
}

// ScanResultHandlerDeps は ScanResultHandler の依存（dig struct-based injection）。
type ScanResultHandlerDeps struct {
	dig.In
	Service *report.Service
	Logger  *slog.Logger
}

// NewScanResultHandler は ScanResultHandler を生成する。
func NewScanResultHandler(d ScanResultHandlerDeps) *ScanResultHandler {
	return &ScanResultHandler{svc: d.Service, logger: d.Logger}
}

// RegisterRoutes はスキャン結果関連のルートを登録する。
func (h *ScanResultHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/scans/:id", h.getState)
	r.GET("/scans/:id/findings", h.getFindings)
}

// getState はスキャンの状態（status＋サマリ）を返す。診断はバックグラウンド実行のため、
// scan が存在する限り 200＋status（queued/running/…）で進捗を伝える。summary は done で非 nil。
//
// @Summary      スキャン状態を取得
// @Description  スキャンの status とサマリ（done は score/band/counts、未完了は summary=null）を返す。進捗ポーリング兼用。
// @Tags         scans
// @Produce      json
// @Param        id   path      string  true  "Scan ID (UUID)"
// @Success      200  {object}  report.ScanState
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /scans/{id} [get]
func (h *ScanResultHandler) getState(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan id"})
		return
	}

	state, err := h.svc.ScanState(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, report.ErrScanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
			return
		}
		h.logger.Error("get scan state failed", "scan_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, state)
}

// getFindings はスキャンの findings 明細を重大度順で返す。findings 0 件（クリーン）は 200＋空配列、
// scan 自体が無い場合のみ 404。
//
// @Summary      スキャンの findings 明細を取得
// @Description  findings を重大度の重い順（Critical→Info）で返す。クリーン（0件）は空配列、scan 不在は 404。
// @Tags         scans
// @Produce      json
// @Param        id   path      string  true  "Scan ID (UUID)"
// @Success      200  {object}  handler.findingsResponse
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /scans/{id}/findings [get]
func (h *ScanResultHandler) getFindings(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan id"})
		return
	}

	findings, err := h.svc.ScanFindings(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, report.ErrScanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
			return
		}
		h.logger.Error("get scan findings failed", "scan_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, findingsResponse{ScanID: id.String(), Findings: findings})
}
