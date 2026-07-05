//go:build integration

package handler_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertScanFull は status / summary_json / created_at を明示してスキャンを挿入する
// （診断履歴の順序を決定的に検証するため created_at を制御する）。summaryJSON が "" なら NULL。
func insertScanFull(t *testing.T, pool *pgxpool.Pool, siteID uuid.UUID, status, summaryJSON string, createdAt time.Time) {
	t.Helper()
	var summary any
	if summaryJSON != "" {
		summary = summaryJSON
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO scans (site_id, status, summary_json, created_at)
		 VALUES ($1, $2, $3::jsonb, $4)`,
		siteID, status, summary, createdAt); err != nil {
		t.Fatalf("insert scan (status=%s): %v", status, err)
	}
}

func TestSiteScansHandlerFlow(t *testing.T) {
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
		code, _ := doJSON(t, r, http.MethodGet, "/sites/not-a-uuid/scans", "")
		if code != http.StatusBadRequest {
			t.Fatalf("code = %d, want 400", code)
		}
	})

	t.Run("unknown site: 200 with empty scans", func(t *testing.T) {
		code, body := doJSON(t, r, http.MethodGet, "/sites/"+uuid.NewString()+"/scans", "")
		if code != http.StatusOK {
			t.Fatalf("code = %d, want 200", code)
		}
		scans, ok := body["scans"].([]any)
		if !ok || len(scans) != 0 {
			t.Errorf("scans = %v, want empty array", body["scans"])
		}
	})

	t.Run("history: newest first with per-scan status and summary", func(t *testing.T) {
		siteID := insertScanTestSite(t, pool, "https://history.example.com", true)
		base := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
		// 古い順に挿入。期待は新しい順（failed → queued → done）。
		insertScanFull(t, pool, siteID, "done",
			`{"findings":{"critical":1,"high":0,"medium":0,"low":0,"info":0,"total":1}}`, base)
		insertScanFull(t, pool, siteID, "queued", "", base.Add(1*time.Hour))
		insertScanFull(t, pool, siteID, "failed", "", base.Add(2*time.Hour))

		code, body := doJSON(t, r, http.MethodGet, "/sites/"+siteID.String()+"/scans", "")
		if code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body=%v)", code, body)
		}
		scans, _ := body["scans"].([]any)
		if len(scans) != 3 {
			t.Fatalf("len(scans) = %d, want 3", len(scans))
		}

		// 新しい順: failed（summary null）→ queued（summary null）→ done（summary あり・score 60）。
		wantStatus := []string{"failed", "queued", "done"}
		for i, ws := range wantStatus {
			s := scans[i].(map[string]any)
			if s["status"] != ws {
				t.Errorf("scans[%d].status = %v, want %s", i, s["status"], ws)
			}
		}
		if scans[0].(map[string]any)["summary"] != nil {
			t.Errorf("failed scan summary = %v, want nil", scans[0].(map[string]any)["summary"])
		}
		doneSummary, ok := scans[2].(map[string]any)["summary"].(map[string]any)
		if !ok {
			t.Fatalf("done scan summary = %v, want object", scans[2].(map[string]any)["summary"])
		}
		if doneSummary["score"].(float64) != 60 {
			t.Errorf("done scan score = %v, want 60", doneSummary["score"])
		}
	})
}
