// Package scanjob は river から scan ジョブを受け取り実行する。
// 実際の Nuclei スキャンは ADR-0002 で worker/internal/engine に実装する。
// 本パッケージは現時点ではプラミング検証用に状態遷移のみを行うスタブ。
package scanjob

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ymd38/goodast/jobs"
	"github.com/ymd38/goodast/worker/internal/db"
)

// Worker は scan ジョブの river.Worker 実装。
type Worker struct {
	river.WorkerDefaults[jobs.ScanArgs]
	queries *db.Queries
	logger  *slog.Logger
}

// NewWorker は scan ジョブワーカーを生成する。
func NewWorker(queries *db.Queries, logger *slog.Logger) *Worker {
	return &Worker{queries: queries, logger: logger}
}

// Work は scan ジョブを処理する。queued→running→done と遷移させる。
// 状態遷移ガード（StartScan/CompleteScan の WHERE）により、既に処理済みの
// ジョブを二重に進めることはなく、その場合 ErrNoRows を返してリトライに委ねる。
func (w *Worker) Work(ctx context.Context, job *river.Job[jobs.ScanArgs]) error {
	scanID, err := uuid.Parse(job.Args.ScanID)
	if err != nil {
		return fmt.Errorf("invalid scan id %q: %w", job.Args.ScanID, err)
	}
	pgID := pgtype.UUID{Bytes: scanID, Valid: true}

	if _, err := w.queries.StartScan(ctx, db.StartScanParams{
		ID:            pgID,
		EngineVersion: pgtype.Text{String: "stub", Valid: true},
	}); err != nil {
		return fmt.Errorf("start scan %s: %w", scanID, err)
	}
	w.logger.Info("scan started", "scan_id", scanID)

	// TODO(ADR-0002): ここで worker/internal/engine の Nuclei スキャンを実行し、
	// findings を保存する。現在はプラミング検証用のスタブ summary を書き込む。
	summary := []byte(`{"placeholder":true,"findings":{"total":0}}`)

	if _, err := w.queries.CompleteScan(ctx, db.CompleteScanParams{
		ID:          pgID,
		SummaryJson: summary,
	}); err != nil {
		return fmt.Errorf("complete scan %s: %w", scanID, err)
	}
	w.logger.Info("scan completed (stub)", "scan_id", scanID)
	return nil
}
