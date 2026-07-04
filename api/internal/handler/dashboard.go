package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/api/internal/report"
)

// DashboardHandler はサイト単位のダッシュボード集計（Goodast Security Score の状態＋遷移）の
// HTTP ハンドラ。
type DashboardHandler struct {
	svc    *report.Service
	logger *slog.Logger
}

// DashboardHandlerDeps は DashboardHandler の依存（dig struct-based injection）。
type DashboardHandlerDeps struct {
	dig.In
	Service *report.Service
	Logger  *slog.Logger
}

// NewDashboardHandler は DashboardHandler を生成する。
func NewDashboardHandler(d DashboardHandlerDeps) *DashboardHandler {
	return &DashboardHandler{svc: d.Service, logger: d.Logger}
}

// RegisterRoutes はサイト単位のレポート系ルート（ダッシュボード集計・診断履歴）を登録する。
func (h *DashboardHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/sites/:id/dashboard", h.get)
	r.GET("/sites/:id/scans", h.listScans)
}

// get はサイトのダッシュボードデータ（最新スコア＋前回差分＋スコア時系列）を返す。
// スキャンが無い（未知サイト含む）場合も 200 で latest=null・history=[] を返す。
func (h *DashboardHandler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	data, err := h.svc.Dashboard(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("dashboard aggregation failed", "site_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, data)
}

// listScans はサイトの診断履歴（全スキャンを新しい順）を返す（§6.5）。
// スキャンが無い（未知サイト含む）場合も 200 で scans=[] を返す。
func (h *DashboardHandler) listScans(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	scans, err := h.svc.SiteScans(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("list site scans failed", "site_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"site_id": id.String(), "scans": scans})
}
