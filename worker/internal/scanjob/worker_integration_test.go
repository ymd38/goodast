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

// TestScanWorker_ProcessesJob は enqueue→dequeue→done の非同期パイプラインを検証する。
// 事前に TEST_DATABASE_URL のDBにマイグレーションが適用されていること。
func TestScanWorker_ProcessesJob(t *testing.T) {
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

	// テストデータ: site + queued scan を直接投入（id は text で受ける）。
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

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	// worker クライアント（ScanWorker 登録）。
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

	// enqueue（api と同じ jobs.ScanArgs 契約を使用）。
	insertOnly, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("insert-only client: %v", err)
	}
	if _, err := insertOnly.Insert(ctx, jobs.ScanArgs{ScanID: scanID}, nil); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if err := workerClient.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = workerClient.Stop(ctx) }()

	// done になるまでポーリング。
	deadline := time.Now().Add(15 * time.Second)
	for {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM scans WHERE id = $1::uuid`, scanID).Scan(&status); err != nil {
			t.Fatalf("query status: %v", err)
		}
		if status == "done" {
			return // 成功
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan did not reach done; last status=%q", status)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
