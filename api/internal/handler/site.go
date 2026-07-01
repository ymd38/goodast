// Package handler は Gin の HTTP レイヤー。service にのみ依存し、バインド・ステータスコード・
// レスポンス整形だけを担う（ビジネスロジックを持たない）。
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/api/internal/site"
)

// SiteHandler はサイト登録・所有確認の HTTP ハンドラ。
type SiteHandler struct {
	svc *site.Service
}

// SiteHandlerDeps は SiteHandler の依存（dig struct-based injection）。
type SiteHandlerDeps struct {
	dig.In
	Service *site.Service
}

// NewSiteHandler は SiteHandler を生成する。
func NewSiteHandler(d SiteHandlerDeps) *SiteHandler {
	return &SiteHandler{svc: d.Service}
}

// RegisterRoutes は site 関連のルートを登録する。
func (h *SiteHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/sites", h.register)
	r.GET("/sites", h.list)
	r.GET("/sites/:id", h.get)
	r.POST("/sites/:id/verify", h.verify)
}

type registerSiteRequest struct {
	Name         string `json:"name" binding:"required"`
	BaseURL      string `json:"base_url" binding:"required,url"`
	VerifyMethod string `json:"verify_method"`
}

type verificationGuide struct {
	Method      string `json:"method"`
	FilePath    string `json:"file_path,omitempty"`
	FileContent string `json:"file_content,omitempty"`
	DNSRecord   string `json:"dns_record,omitempty"`
}

type siteResponse struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	BaseURL           string             `json:"base_url"`
	OwnershipVerified bool               `json:"ownership_verified"`
	VerifyMethod      *string            `json:"verify_method,omitempty"`
	VerifyToken       *string            `json:"verify_token,omitempty"`
	Verification      *verificationGuide `json:"verification,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
}

func (h *SiteHandler) register(c *gin.Context) {
	var req registerSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 方式未指定は初心者向けの file をデフォルトにする。ローカル対象では無視される。
	method := req.VerifyMethod
	if method == "" {
		method = string(site.VerifyMethodFile)
	}
	parsedMethod, err := site.ParseVerifyMethod(method)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.svc.Register(c.Request.Context(), site.RegisterParams{
		Name:    req.Name,
		BaseURL: req.BaseURL,
		Method:  parsedMethod,
	})
	if err != nil {
		switch {
		case errors.Is(err, site.ErrSiteNameTaken):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			// 不正な base_url（スキーマ/ホスト）等の入力エラーは 400。
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, toSiteResponse(created))
}

func (h *SiteHandler) list(c *gin.Context) {
	sites, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sites"})
		return
	}
	resp := make([]siteResponse, 0, len(sites))
	for _, s := range sites {
		resp = append(resp, toSiteResponse(s))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SiteHandler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	s, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		h.writeSiteError(c, err)
		return
	}
	c.JSON(http.StatusOK, toSiteResponse(s))
}

func (h *SiteHandler) verify(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	s, err := h.svc.Verify(c.Request.Context(), id)
	if err != nil {
		h.writeSiteError(c, err)
		return
	}
	c.JSON(http.StatusOK, toSiteResponse(s))
}

// writeSiteError はサービスのドメインエラーを HTTP ステータスへ対応づける。
func (h *SiteHandler) writeSiteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, site.ErrSiteNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, site.ErrVerificationFailed):
		// 所有確認未達（ファイル未設置・TXT不一致等）。クライアント側の設置作業で解決可能。
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

// toSiteResponse はドメイン Site を API レスポンスに整形する。所有確認トークンを持つ
// （＝非ローカルで未確認）場合は設置ガイドを添える。
func toSiteResponse(s site.Site) siteResponse {
	resp := siteResponse{
		ID:                s.ID.String(),
		Name:              s.Name,
		BaseURL:           s.BaseURL,
		OwnershipVerified: s.OwnershipVerified,
		CreatedAt:         s.CreatedAt,
	}
	if s.VerifyMethod != nil {
		m := string(*s.VerifyMethod)
		resp.VerifyMethod = &m
	}
	if s.VerifyToken != nil {
		tok := s.VerifyToken.String()
		resp.VerifyToken = &tok
		resp.Verification = buildGuide(s)
	}
	return resp
}

// buildGuide は所有確認の設置手順を組み立てる（token を持つ site 前提）。
func buildGuide(s site.Site) *verificationGuide {
	token := s.VerifyToken.String()
	g := &verificationGuide{Method: string(*s.VerifyMethod)}
	switch *s.VerifyMethod {
	case site.VerifyMethodFile:
		g.FilePath = "/.well-known/goodast-verify/" + token
		g.FileContent = token
	case site.VerifyMethodDNSTXT:
		g.DNSRecord = "goodast-verify=" + token
	}
	return g
}
