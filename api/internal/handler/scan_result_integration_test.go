//go:build integration

package handler_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ymd38/goodast/api/internal/db"
	"github.com/ymd38/goodast/api/internal/handler"
	"github.com/ymd38/goodast/api/internal/report"
)

func setupScanResultRouter(t *testing.T, pool *pgxpool.Pool) *gin.Engine {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := report.NewService(report.NewRepository(db.New(pool)))
	h := handler.NewScanResultHandler(handler.ScanResultHandlerDeps{Service: svc, Logger: quiet})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

// insertQueuedScan は summary_json を持たない queued スキャンを挿入し scan_id を返す。
func insertQueuedScan(t *testing.T, pool *pgxpool.Pool, siteID uuid.UUID) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO scans (site_id, status) VALUES ($1, 'queued') RETURNING id::text`,
		siteID).Scan(&id); err != nil {
		t.Fatalf("insert queued scan: %v", err)
	}
	return id
}

// insertFinding は findings 1 行を挿入する。
func insertFinding(t *testing.T, pool *pgxpool.Pool, scanID, severity, url string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO findings (scan_id, template_id, title, severity, url)
		 VALUES ($1::uuid, $2, $3, $4, $5)`,
		scanID, "tmpl-"+severity, severity+" issue", severity, url); err != nil {
		t.Fatalf("insert finding: %v", err)
	}
}

func TestScanResultHandlerFlow(t *testing.T) {
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
	r := setupScanResultRouter(t, pool)
	siteID := insertScanTestSite(t, pool, "https://result.example.com", true)

	// --- GET /scans/:id （状態）---
	t.Run("state invalid uuid: 400", func(t *testing.T) {
		code, _ := doJSON(t, r, http.MethodGet, "/scans/not-a-uuid", "")
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d, want 400", code)
		}
	})

	t.Run("state unknown scan: 404", func(t *testing.T) {
		code, _ := doJSON(t, r, http.MethodGet, "/scans/"+uuid.NewString(), "")
		if code != http.StatusNotFound {
			t.Fatalf("code = %d, want 404", code)
		}
	})

	t.Run("state queued: 200 with status and null summary", func(t *testing.T) {
		scanID := insertQueuedScan(t, pool, siteID)
		code, body := doJSON(t, r, http.MethodGet, "/scans/"+scanID, "")
		if code != http.StatusOK {
			t.Fatalf("code = %d, want 200", code)
		}
		if body["status"] != "queued" {
			t.Errorf("status = %v, want queued", body["status"])
		}
		if body["summary"] != nil {
			t.Errorf("summary = %v, want nil (未完了)", body["summary"])
		}
	})

	t.Run("state done: 200 with summary and score", func(t *testing.T) {
		scanID := insertDoneScan(t, pool, siteID,
			`{"critical":1,"high":0,"medium":0,"low":0,"info":0,"total":1}`, time.Now())
		code, body := doJSON(t, r, http.MethodGet, "/scans/"+scanID, "")
		if code != http.StatusOK {
			t.Fatalf("code = %d, want 200", code)
		}
		if body["status"] != "done" {
			t.Errorf("status = %v, want done", body["status"])
		}
		summary, ok := body["summary"].(map[string]any)
		if !ok {
			t.Fatalf("summary = %v, want object", body["summary"])
		}
		if summary["score"].(float64) != 60 || summary["band"] != "caution" || summary["label"] != "要注意" {
			t.Errorf("summary = %v, want score=60 band=caution label=要注意", summary)
		}
	})

	// --- GET /scans/:id/findings （明細）---
	t.Run("findings invalid uuid: 400", func(t *testing.T) {
		code, _ := doJSON(t, r, http.MethodGet, "/scans/not-a-uuid/findings", "")
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d, want 400", code)
		}
	})

	t.Run("findings unknown scan: 404", func(t *testing.T) {
		code, _ := doJSON(t, r, http.MethodGet, "/scans/"+uuid.NewString()+"/findings", "")
		if code != http.StatusNotFound {
			t.Fatalf("code = %d, want 404", code)
		}
	})

	t.Run("findings clean scan: 200 with empty array", func(t *testing.T) {
		scanID := insertDoneScan(t, pool, siteID,
			`{"critical":0,"high":0,"medium":0,"low":0,"info":0,"total":0}`, time.Now())
		code, body := doJSON(t, r, http.MethodGet, "/scans/"+scanID+"/findings", "")
		if code != http.StatusOK {
			t.Fatalf("code = %d, want 200", code)
		}
		fs, ok := body["findings"].([]any)
		if !ok || len(fs) != 0 {
			t.Errorf("findings = %v, want empty array", body["findings"])
		}
	})

	t.Run("findings ordered by severity (Critical first)", func(t *testing.T) {
		scanID := insertDoneScan(t, pool, siteID,
			`{"critical":1,"high":1,"low":1,"medium":0,"info":0,"total":3}`, time.Now())
		// 重い順とは逆順に挿入して、SQL の severity 順が効くことを確認する。
		insertFinding(t, pool, scanID, "Low", "https://result.example.com/a")
		insertFinding(t, pool, scanID, "Critical", "https://result.example.com/b")
		insertFinding(t, pool, scanID, "High", "https://result.example.com/c")

		code, body := doJSON(t, r, http.MethodGet, "/scans/"+scanID+"/findings", "")
		if code != http.StatusOK {
			t.Fatalf("code = %d, want 200", code)
		}
		fs, _ := body["findings"].([]any)
		if len(fs) != 3 {
			t.Fatalf("len(findings) = %d, want 3", len(fs))
		}
		want := []string{"Critical", "High", "Low"}
		for i, w := range want {
			got := fs[i].(map[string]any)["severity"]
			if got != w {
				t.Errorf("findings[%d].severity = %v, want %s", i, got, w)
			}
		}
	})
}
