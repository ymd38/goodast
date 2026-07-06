//go:build integration

package scan_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/ymd38/goodast/api/internal/scan"
	"github.com/ymd38/goodast/jobs"
)

func newTestService(t *testing.T, pool *pgxpool.Pool) *scan.Service {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: quiet})
	if err != nil {
		t.Fatalf("river client: %v", err)
	}
	return scan.NewService(pool, riverClient)
}

func insertSite(t *testing.T, pool *pgxpool.Pool, baseURL string, verified bool) uuid.UUID {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO sites (name, base_url, ownership_verified) VALUES ($1, $2, $3) RETURNING id::text`,
		"itest-"+uuid.NewString(), baseURL, verified).Scan(&id); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	return uuid.MustParse(id)
}

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
	svc := newTestService(t, pool)

	t.Run("verified site: atomic enqueue", func(t *testing.T) {
		siteID := insertSite(t, pool, "https://example.com", true)
		scanID, err := svc.EnqueueScan(ctx, siteID, jobs.PresetStandard)
		if err != nil {
			t.Fatalf("EnqueueScan: %v", err)
		}
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM scans WHERE id = $1`, scanID).Scan(&status); err != nil {
			t.Fatalf("query scan: %v", err)
		}
		if status != "queued" {
			t.Errorf("scan status = %q, want queued", status)
		}
		var jobCount int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM river_job WHERE kind = 'scan' AND args->>'scan_id' = $1`,
			scanID.String()).Scan(&jobCount); err != nil {
			t.Fatalf("query river_job: %v", err)
		}
		if jobCount != 1 {
			t.Errorf("river_job count = %d, want 1", jobCount)
		}
	})

	t.Run("unverified public site: rejected, nothing persisted", func(t *testing.T) {
		siteID := insertSite(t, pool, "https://example.com", false)
		_, err := svc.EnqueueScan(ctx, siteID, jobs.PresetStandard)
		if !errors.Is(err, scan.ErrOwnershipNotVerified) {
			t.Fatalf("err = %v, want ErrOwnershipNotVerified", err)
		}
		var scanCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM scans WHERE site_id = $1`, siteID).Scan(&scanCount); err != nil {
			t.Fatalf("count scans: %v", err)
		}
		if scanCount != 0 {
			t.Errorf("scan rows = %d, want 0 (no scan created for unverified site)", scanCount)
		}
	})

	t.Run("localhost site: enqueues without verification", func(t *testing.T) {
		siteID := insertSite(t, pool, "http://localhost:3000", false)
		if _, err := svc.EnqueueScan(ctx, siteID, jobs.PresetStandard); err != nil {
			t.Fatalf("EnqueueScan (localhost): %v", err)
		}
	})

	t.Run("invalid preset: rejected before persisting", func(t *testing.T) {
		siteID := insertSite(t, pool, "http://localhost:3000", false)
		_, err := svc.EnqueueScan(ctx, siteID, jobs.Preset("bogus"))
		if !errors.Is(err, jobs.ErrInvalidPreset) {
			t.Fatalf("err = %v, want ErrInvalidPreset", err)
		}
	})

	t.Run("empty preset normalizes to standard and persists", func(t *testing.T) {
		siteID := insertSite(t, pool, "http://localhost:3000", false)
		scanID, err := svc.EnqueueScan(ctx, siteID, jobs.Preset(""))
		if err != nil {
			t.Fatalf("EnqueueScan (empty preset): %v", err)
		}
		var preset string
		if err := pool.QueryRow(ctx, `SELECT preset FROM scans WHERE id = $1`, scanID).Scan(&preset); err != nil {
			t.Fatalf("query scan: %v", err)
		}
		if preset != string(jobs.PresetStandard) {
			t.Errorf("preset = %q, want %q", preset, jobs.PresetStandard)
		}
	})
}
