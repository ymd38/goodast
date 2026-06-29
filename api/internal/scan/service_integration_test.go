//go:build integration

package scan_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/ymd38/goodast/api/internal/scan"
)

// TestEnqueueScan は scan 行と river ジョブが1トランザクションで作られる（atomic enqueue）ことを検証する。
// 事前に TEST_DATABASE_URL のDBにマイグレーションが適用されていること。
func TestEnqueueScan(t *testing.T) {
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

	var siteID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sites (name, base_url) VALUES ($1, 'http://example.test') RETURNING id::text`,
		"itest-"+uuid.NewString()).Scan(&siteID); err != nil {
		t.Fatalf("insert site: %v", err)
	}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("river client: %v", err)
	}

	svc := scan.NewService(pool, riverClient)
	scanID, err := svc.EnqueueScan(ctx, uuid.MustParse(siteID))
	if err != nil {
		t.Fatalf("EnqueueScan: %v", err)
	}

	// scan 行が queued で作られている。
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM scans WHERE id = $1`, scanID).Scan(&status); err != nil {
		t.Fatalf("query scan: %v", err)
	}
	if status != "queued" {
		t.Errorf("scan status = %q, want queued", status)
	}

	// 同一 scanID の river ジョブが kind='scan' で投入されている。
	var jobCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM river_job WHERE kind = 'scan' AND args->>'scan_id' = $1`,
		scanID.String()).Scan(&jobCount); err != nil {
		t.Fatalf("query river_job: %v", err)
	}
	if jobCount != 1 {
		t.Errorf("river_job count = %d, want 1", jobCount)
	}
}
