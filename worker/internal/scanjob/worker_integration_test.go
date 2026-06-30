//go:build integration

package scanjob_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/ymd38/goodast/jobs"
	"github.com/ymd38/goodast/worker/internal/db"
	"github.com/ymd38/goodast/worker/internal/engine"
	"github.com/ymd38/goodast/worker/internal/scanjob"
)

// fakeEngine は engine.Engine のテスト用実装。SDK を呼ばずに固定 findings を返す。
// （Nuclei SDK 実体の検証は worker/internal/engine/nuclei の integration テストで行う。）
type fakeEngine struct {
	findings []engine.Finding
}

func (f fakeEngine) Version() string { return "fake/v0" }

func (f fakeEngine) Scan(_ context.Context, _ engine.ScanRequest, onFinding engine.FindingCallback) error {
	for _, fd := range f.findings {
		onFinding(fd)
	}
	return nil
}

// seedSite は site を投入し id を返す。base_url・ownership を指定できる。
func seedSite(t *testing.T, pool *pgxpool.Pool, baseURL string, verified bool) string {
	t.Helper()
	var siteID string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO sites (name, base_url, ownership_verified) VALUES ($1, $2, $3) RETURNING id::text`,
		"itest-"+uuid.NewString(), baseURL, verified).Scan(&siteID); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	return siteID
}

// seedScan は指定 site に scan を投入し、必要なら status を上書きして scan id を返す。
func seedScan(t *testing.T, pool *pgxpool.Pool, siteID, status string) string {
	t.Helper()
	ctx := context.Background()
	var scanID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO scans (site_id) VALUES ($1::uuid) RETURNING id::text`, siteID).Scan(&scanID); err != nil {
		t.Fatalf("insert scan: %v", err)
	}
	if status != "queued" {
		if _, err := pool.Exec(ctx,
			`UPDATE scans SET status = $2, started_at = now() WHERE id = $1::uuid`, scanID, status); err != nil {
			t.Fatalf("set status: %v", err)
		}
	}
	return scanID
}

func waitForStatus(t *testing.T, pool *pgxpool.Pool, scanID, want string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM scans WHERE id = $1::uuid`, scanID).Scan(&status); err != nil {
			t.Fatalf("query status: %v", err)
		}
		if status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan %s did not reach %q; last status=%q", scanID, want, status)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func TestScanWorker(t *testing.T) {
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

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := fakeEngine{findings: []engine.Finding{
		{TemplateID: "t-high", Title: "High issue", Severity: engine.SeverityHigh, URL: "http://localhost/x", CWE: "CWE-79"},
		{TemplateID: "t-low", Title: "Low issue", Severity: engine.SeverityLow, URL: "http://localhost/y"},
	}}

	workers := river.NewWorkers()
	river.AddWorker(workers, scanjob.NewWorker(db.New(pool), eng, quiet))
	workerClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
		Logger:  quiet,
	})
	if err != nil {
		t.Fatalf("worker client: %v", err)
	}
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if err := workerClient.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = workerClient.Stop(ctx) }()

	// queued ジョブが dequeue され done になり、findings と summary が保存される。
	t.Run("queued scan completes and persists findings", func(t *testing.T) {
		siteID := seedSite(t, pool, "http://localhost:3000", false)
		scanID := seedScan(t, pool, siteID, "queued")
		if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID}, nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForStatus(t, pool, scanID, "done")

		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM findings WHERE scan_id = $1::uuid`, scanID).Scan(&count); err != nil {
			t.Fatalf("count findings: %v", err)
		}
		if count != 2 {
			t.Errorf("findings count = %d, want 2", count)
		}
		var total int
		if err := pool.QueryRow(ctx,
			`SELECT (summary_json->'findings'->>'total')::int FROM scans WHERE id = $1::uuid`, scanID).Scan(&total); err != nil {
			t.Fatalf("read summary: %v", err)
		}
		if total != 2 {
			t.Errorf("summary total = %d, want 2", total)
		}
	})

	// 既に running（途中失敗→再配送を模擬）でも冪等に done まで進む。
	t.Run("running scan resumes idempotently", func(t *testing.T) {
		siteID := seedSite(t, pool, "http://localhost:3000", false)
		scanID := seedScan(t, pool, siteID, "running")
		if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID}, nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForStatus(t, pool, scanID, "done")
	})

	// 公開ホストで所有未確認なら defense-in-depth で failed（ADR-0004）。
	t.Run("unverified public site is failed", func(t *testing.T) {
		siteID := seedSite(t, pool, "https://example.com", false)
		scanID := seedScan(t, pool, siteID, "queued")
		if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID}, nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForStatus(t, pool, scanID, "failed")
	})
}
