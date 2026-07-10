//go:build integration

package handler_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/ymd38/goodast/api/internal/handler"
	"github.com/ymd38/goodast/api/internal/scan"
)

func setupScanRouter(t *testing.T, pool *pgxpool.Pool) *gin.Engine {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("river client: %v", err)
	}
	svc := scan.NewService(pool, riverClient)
	h := handler.NewScanHandler(handler.ScanHandlerDeps{Service: svc, Logger: quiet})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

func insertScanTestSite(t *testing.T, pool *pgxpool.Pool, baseURL string, verified bool) uuid.UUID {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		// origin は本テストの検証対象外（scan 受付は base_url を見る）。NOT NULL/UNIQUE を
		// 満たすため行ごとに一意な synthetic 値を入れる。
		`INSERT INTO sites (name, base_url, origin, ownership_verified) VALUES ($1, $2, 'itest-'||gen_random_uuid()::text, $3) RETURNING id::text`,
		"htest-"+uuid.NewString(), baseURL, verified).Scan(&id); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	return uuid.MustParse(id)
}

func TestScanHandlerFlow(t *testing.T) {
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
	r := setupScanRouter(t, pool)

	t.Run("verified site: 202 with queued scan and river job", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "https://example.com", true)
		code, body := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"`+siteID.String()+`"}`)
		if code != http.StatusAccepted {
			t.Fatalf("start scan: code=%d body=%v", code, body)
		}
		if body["status"] != "queued" {
			t.Errorf("status = %v, want queued", body["status"])
		}
		scanID, _ := body["scan_id"].(string)
		if scanID == "" {
			t.Fatal("missing scan_id in response")
		}

		var dbStatus string
		if err := pool.QueryRow(ctx, `SELECT status FROM scans WHERE id = $1`, scanID).Scan(&dbStatus); err != nil {
			t.Fatalf("query scan: %v", err)
		}
		if dbStatus != "queued" {
			t.Errorf("scan status = %q, want queued", dbStatus)
		}
		var jobCount int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM river_job WHERE kind = 'scan' AND args->>'scan_id' = $1`,
			scanID).Scan(&jobCount); err != nil {
			t.Fatalf("query river_job: %v", err)
		}
		if jobCount != 1 {
			t.Errorf("river_job count = %d, want 1", jobCount)
		}
	})

	t.Run("unverified public site: 403 (ADR-0004)", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "https://example.com", false)
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"`+siteID.String()+`"}`); code != http.StatusForbidden {
			t.Errorf("unverified: code=%d want 403", code)
		}
	})

	t.Run("localhost site: 202 without verification", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "http://localhost:3000", false)
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"`+siteID.String()+`"}`); code != http.StatusAccepted {
			t.Errorf("localhost: code=%d want 202", code)
		}
	})

	t.Run("concurrent scan on same site: 409", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "https://example.com", true)
		// 1本目は受理（queued）。
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"`+siteID.String()+`"}`); code != http.StatusAccepted {
			t.Fatalf("first scan: code=%d want 202", code)
		}
		// 2本目は実行中（queued）が居るため 409 Conflict。
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"`+siteID.String()+`"}`); code != http.StatusConflict {
			t.Errorf("concurrent scan: code=%d want 409", code)
		}
		// 完了扱いにすると再度受理される（done は active でないため）。
		if _, err := pool.Exec(ctx, `UPDATE scans SET status = 'done' WHERE site_id = $1`,
			siteID.String()); err != nil {
			t.Fatalf("mark done: %v", err)
		}
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"`+siteID.String()+`"}`); code != http.StatusAccepted {
			t.Errorf("re-scan after done: code=%d want 202", code)
		}
	})

	t.Run("missing site: 404", func(t *testing.T) {
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"`+uuid.NewString()+`"}`); code != http.StatusNotFound {
			t.Errorf("missing site: code=%d want 404", code)
		}
	})

	t.Run("invalid site_id: 400", func(t *testing.T) {
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"not-a-uuid"}`); code != http.StatusBadRequest {
			t.Errorf("bad uuid: code=%d want 400", code)
		}
	})

	t.Run("missing body field: 400", func(t *testing.T) {
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{}`); code != http.StatusBadRequest {
			t.Errorf("empty body: code=%d want 400", code)
		}
	})

	t.Run("invalid preset: 400", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "https://example.com", true)
		if code, _ := doJSON(t, r, http.MethodPost, "/scans", `{"site_id":"`+siteID.String()+`","preset":"bogus"}`); code != http.StatusBadRequest {
			t.Errorf("invalid preset: code=%d want 400", code)
		}
	})
}
