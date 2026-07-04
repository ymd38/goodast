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

func setupDashboardRouter(t *testing.T, pool *pgxpool.Pool) *gin.Engine {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := report.NewRepository(db.New(pool))
	svc := report.NewService(repo)
	h := handler.NewDashboardHandler(handler.DashboardHandlerDeps{Service: svc, Logger: quiet})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

// insertDoneScan は done スキャンを summary_json / finished_at 付きで挿入し scan_id を返す。
func insertDoneScan(t *testing.T, pool *pgxpool.Pool, siteID uuid.UUID, summaryJSON string, finishedAt time.Time) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO scans (site_id, status, summary_json, finished_at)
		 VALUES ($1, 'done', $2::jsonb, $3) RETURNING id::text`,
		siteID, summaryJSON, finishedAt).Scan(&id); err != nil {
		t.Fatalf("insert done scan: %v", err)
	}
	return id
}

func TestDashboardHandlerFlow(t *testing.T) {
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
	r := setupDashboardRouter(t, pool)

	t.Run("invalid uuid: 400", func(t *testing.T) {
		code, _ := doJSON(t, r, http.MethodGet, "/sites/not-a-uuid/dashboard", "")
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d, want 400", code)
		}
	})

	t.Run("no scans: 200 with null latest and empty history", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "https://empty.example.com", true)
		code, body := doJSON(t, r, http.MethodGet, "/sites/"+siteID.String()+"/dashboard", "")
		if code != http.StatusOK {
			t.Fatalf("code = %d, want 200", code)
		}
		if body["latest"] != nil {
			t.Errorf("latest = %v, want nil", body["latest"])
		}
		hist, ok := body["history"].([]any)
		if !ok || len(hist) != 0 {
			t.Errorf("history = %v, want empty array", body["history"])
		}
	})

	t.Run("done scans: ordered history, latest score and delta", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "https://scored.example.com", true)
		base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
		// 古い順に挿入（High 1件=90）→（Critical 1件=60）。delta は 60-90 = -30。
		insertDoneScan(t, pool, siteID, `{"critical":0,"high":1,"medium":0,"low":0,"info":0,"total":1}`, base)
		insertDoneScan(t, pool, siteID, `{"critical":1,"high":0,"medium":0,"low":0,"info":0,"total":1}`, base.Add(24*time.Hour))
		// 除外されるべきスキャン: queued（未完了）と done だが summary_json NULL。
		if _, err := pool.Exec(ctx, `INSERT INTO scans (site_id, status) VALUES ($1, 'queued')`, siteID); err != nil {
			t.Fatalf("insert queued scan: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO scans (site_id, status, finished_at) VALUES ($1, 'done', now())`, siteID); err != nil {
			t.Fatalf("insert done scan without summary: %v", err)
		}

		code, body := doJSON(t, r, http.MethodGet, "/sites/"+siteID.String()+"/dashboard", "")
		if code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body=%v)", code, body)
		}

		hist, _ := body["history"].([]any)
		if len(hist) != 2 {
			t.Fatalf("len(history) = %d, want 2 (queued / summary-less done は除外)", len(hist))
		}
		// 昇順（左→右）: 90 → 60。
		if got := hist[0].(map[string]any)["score"].(float64); got != 90 {
			t.Errorf("history[0].score = %v, want 90", got)
		}
		if got := hist[1].(map[string]any)["score"].(float64); got != 60 {
			t.Errorf("history[1].score = %v, want 60", got)
		}

		latest, ok := body["latest"].(map[string]any)
		if !ok {
			t.Fatalf("latest = %v, want object", body["latest"])
		}
		if latest["score"].(float64) != 60 {
			t.Errorf("latest.score = %v, want 60", latest["score"])
		}
		if latest["band"] != "caution" || latest["label"] != "要注意" {
			t.Errorf("latest band/label = %v/%v, want caution/要注意", latest["band"], latest["label"])
		}
		if latest["delta"].(float64) != -30 {
			t.Errorf("latest.delta = %v, want -30", latest["delta"])
		}
	})

	t.Run("ordered by finished_at, not created_at (backfill safe)", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "https://ordering.example.com", true)
		base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
		// A: created_at は後だが finished_at は先（=先に完了）。High 1件 → 90。
		if _, err := pool.Exec(ctx,
			`INSERT INTO scans (site_id, status, summary_json, created_at, finished_at)
			 VALUES ($1, 'done', $2::jsonb, $3, $4)`,
			siteID, `{"critical":0,"high":1,"medium":0,"low":0,"info":0,"total":1}`,
			base.Add(10*time.Hour), base.Add(1*time.Hour)); err != nil {
			t.Fatalf("insert scan A: %v", err)
		}
		// B: created_at は先だが finished_at は後（=後に完了）。Critical 1件 → 60。
		if _, err := pool.Exec(ctx,
			`INSERT INTO scans (site_id, status, summary_json, created_at, finished_at)
			 VALUES ($1, 'done', $2::jsonb, $3, $4)`,
			siteID, `{"critical":1,"high":0,"medium":0,"low":0,"info":0,"total":1}`,
			base.Add(1*time.Hour), base.Add(10*time.Hour)); err != nil {
			t.Fatalf("insert scan B: %v", err)
		}

		_, body := doJSON(t, r, http.MethodGet, "/sites/"+siteID.String()+"/dashboard", "")
		hist, _ := body["history"].([]any)
		if len(hist) != 2 {
			t.Fatalf("len(history) = %d, want 2", len(hist))
		}
		// finished_at 昇順なら A(90) → B(60)。created_at 昇順だと B(60) → A(90) になり不一致。
		if got := hist[0].(map[string]any)["score"].(float64); got != 90 {
			t.Errorf("history[0].score = %v, want 90 (finished_at 先の A)", got)
		}
		latest, _ := body["latest"].(map[string]any)
		if latest["score"].(float64) != 60 {
			t.Errorf("latest.score = %v, want 60 (finished_at 後の B)", latest["score"])
		}
	})
}
