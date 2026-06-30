// Package scanjob は river から scan ジョブを受け取り実行する。
// 実際の Nuclei スキャンは ADR-0002 で worker/internal/engine に実装する。
// 本パッケージは現時点ではプラミング検証用に状態遷移のみを行うスタブ。
package scanjob

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
//
// リトライ耐性（冪等性）: StartScan/CompleteScan は状態ガード付きで、合致しない場合
// ErrNoRows を返す。途中まで進んだジョブが再配送・再起動された場合は現在の状態を見て
// 続行 or 完了済みスキップに振り分け、scans が running のまま詰まらないようにする。
func (w *Worker) Work(ctx context.Context, job *river.Job[jobs.ScanArgs]) error {
	scanID, err := uuid.Parse(job.Args.ScanID)
	if err != nil {
		return fmt.Errorf("invalid scan id %q: %w", job.Args.ScanID, err)
	}
	pgID := pgtype.UUID{Bytes: scanID, Valid: true}

	// queued -> running。ErrNoRows は「既に queued ではない」= 再試行の可能性。
	_, err = w.queries.StartScan(ctx, db.StartScanParams{
		ID:            pgID,
		EngineVersion: pgtype.Text{String: "stub", Valid: true},
	})
	switch {
	case err == nil:
		w.logger.Info("scan started", "scan_id", scanID)
	case errors.Is(err, pgx.ErrNoRows):
		done, derr := w.resumeOrSkip(ctx, scanID, pgID)
		if derr != nil {
			return derr
		}
		if done {
			return nil // 既に終了済み。ジョブは完了扱い。
		}
		// running だった: そのまま続行（再試行）。
	default:
		return fmt.Errorf("start scan %s: %w", scanID, err)
	}

	// TODO(ADR-0002): ここで worker/internal/engine の Nuclei スキャンを実行し、
	// findings を保存する。現在はプラミング検証用のスタブ summary を書き込む。
	summary := []byte(`{"placeholder":true,"findings":{"total":0}}`)

	// running -> done。既に done/failed 等で running でなければ冪等に成功扱いにする。
	if _, err := w.queries.CompleteScan(ctx, db.CompleteScanParams{
		ID:          pgID,
		SummaryJson: summary,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.logger.Info("scan no longer running at completion; treating as done", "scan_id", scanID)
			return nil
		}
		return fmt.Errorf("complete scan %s: %w", scanID, err)
	}
	w.logger.Info("scan completed (stub)", "scan_id", scanID)
	return nil
}

// resumeOrSkip は StartScan が ErrNoRows のとき、現在の scan 状態から処理を振り分ける。
// 戻り値 done=true は「終端状態なのでジョブを完了扱いにしてよい」を表す。
func (w *Worker) resumeOrSkip(ctx context.Context, scanID uuid.UUID, pgID pgtype.UUID) (done bool, err error) {
	scan, err := w.queries.GetScan(ctx, pgID)
	if err != nil {
		return false, fmt.Errorf("get scan %s: %w", scanID, err)
	}
	switch scan.Status {
	case "running":
		w.logger.Info("scan already running; resuming", "scan_id", scanID)
		return false, nil
	case "done", "failed":
		w.logger.Info("scan already terminal; skipping", "scan_id", scanID, "status", scan.Status)
		return true, nil
	default:
		return false, fmt.Errorf("scan %s in unexpected state %q", scanID, scan.Status)
	}
}
