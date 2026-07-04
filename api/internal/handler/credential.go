package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/api/internal/credential"
	"github.com/ymd38/goodast/secrets"
)

// CredentialHandler はサイトの session 認証情報（持ち込み Cookie/Bearer）の HTTP ハンドラ。
// 認証情報の生値はログにもレスポンスにも一切出さない（ADR-0003）。
type CredentialHandler struct {
	svc    *credential.Service
	logger *slog.Logger
}

// CredentialHandlerDeps は CredentialHandler の依存（dig struct-based injection）。
type CredentialHandlerDeps struct {
	dig.In
	Service *credential.Service
	Logger  *slog.Logger
}

// NewCredentialHandler は CredentialHandler を生成する。
func NewCredentialHandler(d CredentialHandlerDeps) *CredentialHandler {
	return &CredentialHandler{svc: d.Service, logger: d.Logger}
}

// RegisterRoutes は credential 関連のルートを登録する。
func (h *CredentialHandler) RegisterRoutes(r gin.IRouter) {
	r.PUT("/sites/:id/credentials", h.set)
	r.DELETE("/sites/:id/credentials", h.clear)
	r.GET("/sites/:id/credentials", h.get)
}

type headerInput struct {
	Name  string `json:"name" binding:"required"`
	Value string `json:"value" binding:"required"`
}

type setCredentialsRequest struct {
	Headers []headerInput `json:"headers" binding:"required,min=1,dive"`
}

type credentialStatusResponse struct {
	AuthMode   string  `json:"auth_mode"`
	Configured bool    `json:"configured"`
	CreatedAt  *string `json:"created_at,omitempty"`
}

// set はサイトの session 認証情報（持ち込み Cookie/Bearer）を暗号化保存する。
//
// @Summary      認証情報を設定
// @Description  持ち込みセッション（ヘッダ）をアプリ層で暗号化保存（ADR-0003）。生値はレスポンスに含めない。
// @Tags         credentials
// @Accept       json
// @Produce      json
// @Param        id       path      string                  true  "Site ID (UUID)"
// @Param        request  body      setCredentialsRequest   true  "認証ヘッダ"
// @Success      200      {object}  credentialStatusResponse
// @Failure      400      {object}  handler.ErrorResponse
// @Failure      404      {object}  handler.ErrorResponse
// @Failure      500      {object}  handler.ErrorResponse
// @Router       /sites/{id}/credentials [put]
func (h *CredentialHandler) set(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	// 認証情報を含むためボディはログしない。バインドエラーの詳細のみ返す。
	var req setCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.SetSession(c.Request.Context(), id, toHeaders(req.Headers)); err != nil {
		h.writeCredentialError(c, err)
		return
	}
	c.JSON(http.StatusOK, credentialStatusResponse{AuthMode: "session", Configured: true})
}

// clear はサイトの認証情報を削除する（未認証スキャンに戻す）。
//
// @Summary      認証情報を削除
// @Tags         credentials
// @Param        id   path  string  true  "Site ID (UUID)"
// @Success      204  "No Content"
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /sites/{id}/credentials [delete]
func (h *CredentialHandler) clear(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	if err := h.svc.Clear(c.Request.Context(), id); err != nil {
		h.writeCredentialError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// get は認証情報の設定状況（マスク・生値なし）を返す。
//
// @Summary      認証情報の状態を取得
// @Description  auth_mode と configured を返す（生値は含めない）。
// @Tags         credentials
// @Produce      json
// @Param        id   path      string  true  "Site ID (UUID)"
// @Success      200  {object}  credentialStatusResponse
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /sites/{id}/credentials [get]
func (h *CredentialHandler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	status, err := h.svc.GetStatus(c.Request.Context(), id)
	if err != nil {
		h.writeCredentialError(c, err)
		return
	}
	c.JSON(http.StatusOK, credentialStatusResponse{
		AuthMode:   status.AuthMode,
		Configured: status.Configured,
		CreatedAt:  status.CreatedAt,
	})
}

// writeCredentialError はサービス/ドメインのエラーを HTTP ステータスへ対応づける。
func (h *CredentialHandler) writeCredentialError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, credential.ErrSiteNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, secrets.ErrNoHeaders), errors.Is(err, secrets.ErrInvalidHeader):
		// 不正なヘッダ（空・CR/LF 等）はクライアント入力の問題。生値は返さない。
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		h.logger.Error("credential operation failed", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

// toHeaders はリクエスト入力を secrets.Headers に変換する。
func toHeaders(in []headerInput) secrets.Headers {
	out := make(secrets.Headers, 0, len(in))
	for _, h := range in {
		out = append(out, secrets.Header{Name: h.Name, Value: h.Value})
	}
	return out
}
