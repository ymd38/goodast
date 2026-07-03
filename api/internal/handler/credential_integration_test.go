//go:build integration

package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ymd38/goodast/api/internal/credential"
	"github.com/ymd38/goodast/api/internal/db"
	"github.com/ymd38/goodast/api/internal/handler"
	"github.com/ymd38/goodast/secrets"
)

func setupCredentialRouter(t *testing.T, pool *pgxpool.Pool) *gin.Engine {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	cipher, err := secrets.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	repo := credential.NewRepository(db.New(pool))
	svc := credential.NewService(credential.ServiceDeps{Repo: repo, Cipher: cipher, Logger: quiet})
	h := handler.NewCredentialHandler(handler.CredentialHandlerDeps{Service: svc, Logger: quiet})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

func TestCredentialHandlerFlow(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	r := setupCredentialRouter(t, pool)

	siteID := insertScanTestSite(t, pool, "https://example.com", true)
	base := "/sites/" + siteID.String() + "/credentials"
	const secret = "session=super-secret-token-123"

	// 初期状態: 認証情報なし。
	if code, body := doJSON(t, r, http.MethodGet, base, ""); code != http.StatusOK ||
		body["auth_mode"] != "none" || body["configured"] != false {
		t.Fatalf("initial GET: code=%d body=%v", code, body)
	}

	// session 設定。
	setBody := `{"headers":[{"name":"Cookie","value":"` + secret + `"},{"name":"Authorization","value":"Bearer abc"}]}`
	if code, body := doJSON(t, r, http.MethodPut, base, setBody); code != http.StatusOK ||
		body["auth_mode"] != "session" || body["configured"] != true {
		t.Fatalf("PUT session: code=%d body=%v", code, body)
	}

	// DB 検証: enc_headers は暗号化され平文を含まない。auth_mode=session。
	var authMode string
	var enc []byte
	if err := pool.QueryRow(ctx,
		`SELECT auth_mode, enc_headers FROM scan_credentials WHERE site_id = $1`, siteID).Scan(&authMode, &enc); err != nil {
		t.Fatalf("query credentials: %v", err)
	}
	if authMode != "session" {
		t.Errorf("auth_mode = %q, want session", authMode)
	}
	if len(enc) == 0 || bytes.Contains(enc, []byte(secret)) || bytes.Contains(enc, []byte("Bearer abc")) {
		t.Error("enc_headers is empty or leaks plaintext")
	}

	// GET: マスク状態（値・ヘッダ名を返さない）。
	code, body := doJSON(t, r, http.MethodGet, base, "")
	if code != http.StatusOK || body["auth_mode"] != "session" || body["configured"] != true || body["created_at"] == nil {
		t.Fatalf("GET session: code=%d body=%v", code, body)
	}
	if _, leaked := body["headers"]; leaked {
		t.Error("GET response must not include headers")
	}

	// 更新（upsert）。
	if code, _ := doJSON(t, r, http.MethodPut, base,
		`{"headers":[{"name":"Cookie","value":"v2"}]}`); code != http.StatusOK {
		t.Errorf("PUT update: code=%d want 200", code)
	}

	// 削除 → none。
	if code, _ := doJSON(t, r, http.MethodDelete, base, ""); code != http.StatusNoContent {
		t.Errorf("DELETE: code=%d want 204", code)
	}
	if code, b := doJSON(t, r, http.MethodGet, base, ""); code != http.StatusOK || b["auth_mode"] != "none" {
		t.Errorf("GET after delete: code=%d body=%v", code, b)
	}
	// 削除は冪等。
	if code, _ := doJSON(t, r, http.MethodDelete, base, ""); code != http.StatusNoContent {
		t.Errorf("DELETE idempotent: code=%d want 204", code)
	}

	// 不正入力: 空ヘッダ配列 / CR/LF 値 → 400。
	if code, _ := doJSON(t, r, http.MethodPut, base, `{"headers":[]}`); code != http.StatusBadRequest {
		t.Errorf("empty headers: code=%d want 400", code)
	}
	if code, _ := doJSON(t, r, http.MethodPut, base,
		`{"headers":[{"name":"Cookie","value":"a\r\nInjected: 1"}]}`); code != http.StatusBadRequest {
		t.Errorf("CRLF value: code=%d want 400", code)
	}

	// 旧データ/外部挿入で auth_mode='none' 行が残っても configured=false（整合性・Qodo #1）。
	noneSite := insertScanTestSite(t, pool, "https://none.example.com", true)
	if _, err := pool.Exec(ctx,
		`INSERT INTO scan_credentials (site_id, auth_mode, enc_headers) VALUES ($1, 'none', NULL)`, noneSite); err != nil {
		t.Fatalf("insert none row: %v", err)
	}
	if code, b := doJSON(t, r, http.MethodGet, "/sites/"+noneSite.String()+"/credentials", ""); code != http.StatusOK ||
		b["auth_mode"] != "none" || b["configured"] != false {
		t.Errorf("none-row GET: code=%d body=%v (want none/false)", code, b)
	}

	// site 不在 → 404（各メソッド）。不正 uuid → 400。
	missing := "/sites/" + uuid.NewString() + "/credentials"
	if code, _ := doJSON(t, r, http.MethodGet, missing, ""); code != http.StatusNotFound {
		t.Errorf("GET missing site: code=%d want 404", code)
	}
	if code, _ := doJSON(t, r, http.MethodPut, missing,
		`{"headers":[{"name":"Cookie","value":"v"}]}`); code != http.StatusNotFound {
		t.Errorf("PUT missing site: code=%d want 404", code)
	}
	if code, _ := doJSON(t, r, http.MethodDelete, missing, ""); code != http.StatusNotFound {
		t.Errorf("DELETE missing site: code=%d want 404", code)
	}
	if code, _ := doJSON(t, r, http.MethodGet, "/sites/not-a-uuid/credentials", ""); code != http.StatusBadRequest {
		t.Errorf("GET bad uuid: code=%d want 400", code)
	}
}
