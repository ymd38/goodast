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
	"github.com/ymd38/goodast/worker/internal/scanjob"
)

// seedScan は site + scan を投入し、必要なら status を上書きして scan id を返す。
func seedScan(t *testing.T, pool *pgxpool.Pool, status string) string {
	t.Helper()
	ctx := context.Background()
	var siteID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sites (name, base_url) VALUES ($1, 'http://example.test') RETURNING id::text`,
		"itest-"+uuid.NewString()).Scan(&siteID); err != nil {
		t.Fatalf("insert site: %v", err)
	}
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

func waitForDone(t *testing.T, pool *pgxpool.Pool, scanID string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM scans WHERE id = $1::uuid`, scanID).Scan(&status); err != nil {
			t.Fatalf("query status: %v", err)
		}
		if status == "done" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan %s did not reach done; last status=%q", scanID, status)
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
	workers := river.NewWorkers()
	river.AddWorker(workers, scanjob.NewWorker(db.New(pool), quiet))
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

	// queued ジョブが dequeue され done になる（基本パイプライン）。
	t.Run("queued scan completes", func(t *testing.T) {
		scanID := seedScan(t, pool, "queued")
		if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID}, nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForDone(t, pool, scanID)
	})

	// 既に running（途中失敗→再配送を模擬）でも冪等に done まで進む（Finding 2 の検証）。
	t.Run("running scan resumes idempotently", func(t *testing.T) {
		scanID := seedScan(t, pool, "running")
		if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID}, nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForDone(t, pool, scanID)
	})
}
