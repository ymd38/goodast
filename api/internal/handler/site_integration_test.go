//go:build integration

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ymd38/goodast/api/internal/db"
	"github.com/ymd38/goodast/api/internal/handler"
	"github.com/ymd38/goodast/api/internal/site"
)

// toggleHTTP は所有確認ファイル取得を差し替える fake。status/body を切り替えて成功/失敗を制御する。
type toggleHTTP struct {
	status int
	body   string
}

func (t *toggleHTTP) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: t.status, Body: io.NopCloser(bytes.NewBufferString(t.body))}, nil
}

type nopDNS struct{}

func (nopDNS) LookupTXT(context.Context, string) ([]string, error) { return nil, nil }

func setupRouter(t *testing.T, pool *pgxpool.Pool, fh *toggleHTTP) *gin.Engine {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := site.NewRepository(db.New(pool))
	verifier := site.NewVerifier(fh, nopDNS{})
	svc := site.NewService(site.ServiceDeps{Repo: repo, Verifier: verifier, Logger: quiet})
	h := handler.NewSiteHandler(handler.SiteHandlerDeps{Service: svc})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]any
	if w.Body.Len() > 0 && w.Body.Bytes()[0] == '{' {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal %s %s: %v (body=%s)", method, path, err, w.Body.String())
		}
	}
	return w.Code, out
}

func TestSiteHandlerFlow(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	fh := &toggleHTTP{status: http.StatusOK}
	r := setupRouter(t, pool, fh)

	publicName := "corp-" + uuid.NewString()
	localName := "local-" + uuid.NewString()

	// 非ローカル登録: トークン + 設置ガイドが返り、未確認。
	code, body := doJSON(t, r, http.MethodPost, "/sites",
		`{"name":"`+publicName+`","base_url":"https://example.com","verify_method":"file"}`)
	if code != http.StatusCreated {
		t.Fatalf("register public: code=%d body=%v", code, body)
	}
	if body["ownership_verified"] != false {
		t.Errorf("public site should be unverified: %v", body["ownership_verified"])
	}
	token, _ := body["verify_token"].(string)
	if token == "" {
		t.Fatal("public site missing verify_token")
	}
	publicID, _ := body["id"].(string)

	// 同名は 409。
	if code, _ := doJSON(t, r, http.MethodPost, "/sites",
		`{"name":"`+publicName+`","base_url":"https://example.com"}`); code != http.StatusConflict {
		t.Errorf("duplicate name: code=%d want 409", code)
	}

	// ローカル登録: トークンなし。
	code, lbody := doJSON(t, r, http.MethodPost, "/sites",
		`{"name":"`+localName+`","base_url":"http://localhost:3000"}`)
	if code != http.StatusCreated {
		t.Fatalf("register local: code=%d body=%v", code, lbody)
	}
	if _, ok := lbody["verify_token"]; ok {
		t.Error("local site should not carry a verify_token")
	}
	localID, _ := lbody["id"].(string)

	// バリデーション: 不正 base_url は 400（gin バインディング）。
	if code, _ := doJSON(t, r, http.MethodPost, "/sites",
		`{"name":"x-`+uuid.NewString()+`","base_url":"not a url"}`); code != http.StatusBadRequest {
		t.Errorf("invalid base_url: code=%d want 400", code)
	}
	// service 層の ErrInvalidBaseURL 分類: バインディングは通るが scheme 不正 → 400。
	if code, _ := doJSON(t, r, http.MethodPost, "/sites",
		`{"name":"x-`+uuid.NewString()+`","base_url":"ftp://example.com"}`); code != http.StatusBadRequest {
		t.Errorf("ftp scheme: code=%d want 400 (ErrInvalidBaseURL)", code)
	}

	// 一覧。
	req := httptest.NewRequest(http.MethodGet, "/sites", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("list: code=%d want 200", w.Code)
	}

	// 取得: 正常 / 不正ID(400) / 不在(404)。
	if code, _ := doJSON(t, r, http.MethodGet, "/sites/"+publicID, ""); code != http.StatusOK {
		t.Errorf("get public: code=%d want 200", code)
	}
	if code, _ := doJSON(t, r, http.MethodGet, "/sites/not-a-uuid", ""); code != http.StatusBadRequest {
		t.Errorf("get bad id: code=%d want 400", code)
	}
	if code, _ := doJSON(t, r, http.MethodGet, "/sites/"+uuid.NewString(), ""); code != http.StatusNotFound {
		t.Errorf("get missing: code=%d want 404", code)
	}

	// ローカル所有確認: 確認不要で即 verified。
	if code, vb := doJSON(t, r, http.MethodPost, "/sites/"+localID+"/verify", ""); code != http.StatusOK || vb["ownership_verified"] != true {
		t.Errorf("verify local: code=%d verified=%v", code, vb["ownership_verified"])
	}

	// 非ローカル所有確認 失敗: ファイル未設置(404)→422。
	fh.status = http.StatusNotFound
	if code, _ := doJSON(t, r, http.MethodPost, "/sites/"+publicID+"/verify", ""); code != http.StatusUnprocessableEntity {
		t.Errorf("verify fail: code=%d want 422", code)
	}

	// 非ローカル所有確認 成功: ファイルにトークンを設置(200)→200 verified。
	fh.status = http.StatusOK
	fh.body = token
	if code, vb := doJSON(t, r, http.MethodPost, "/sites/"+publicID+"/verify", ""); code != http.StatusOK || vb["ownership_verified"] != true {
		t.Errorf("verify success: code=%d verified=%v", code, vb["ownership_verified"])
	}

	// 確認済みは冪等（再度 200 verified）。
	if code, vb := doJSON(t, r, http.MethodPost, "/sites/"+publicID+"/verify", ""); code != http.StatusOK || vb["ownership_verified"] != true {
		t.Errorf("verify idempotent: code=%d verified=%v", code, vb["ownership_verified"])
	}

	// 確認不正ID。
	if code, _ := doJSON(t, r, http.MethodPost, "/sites/not-a-uuid/verify", ""); code != http.StatusBadRequest {
		t.Errorf("verify bad id: code=%d want 400", code)
	}
}
