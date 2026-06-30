// Package scanjob は river から scan ジョブを受け取り、engine で実スキャンを実行する。
// Nuclei SDK そのものは worker/internal/engine/nuclei に隔離され（ADR-0002）、本パッケージは
// engine.Engine インターフェース越しに呼び出すため SDK には直接依存しない。
package scanjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ymd38/goodast/jobs"
	"github.com/ymd38/goodast/worker/internal/db"
	"github.com/ymd38/goodast/worker/internal/engine"
)

// Worker は scan ジョブの river.Worker 実装。
type Worker struct {
	river.WorkerDefaults[jobs.ScanArgs]
	queries *db.Queries
	engine  engine.Engine
	logger  *slog.Logger
}

// NewWorker は scan ジョブワーカーを生成する。
func NewWorker(queries *db.Queries, eng engine.Engine, logger *slog.Logger) *Worker {
	return &Worker{queries: queries, engine: eng, logger: logger}
}

// scanSummary は scans.summary_json に書き込むダッシュボード描画用の集計データ。
// スコア計算は api 側 report に集約するため、ここでは件数のみ持つ。
type scanSummary struct {
	Findings engine.Summary `json:"findings"`
}

// Work は scan ジョブを処理する。queued→running→（Nuclei 実行）→done と遷移させる。
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
		EngineVersion: pgtype.Text{String: w.engine.Version(), Valid: true},
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

	return w.runScan(ctx, scanID, pgID)
}

// runScan はスキャン対象をロードし、ガードレールを確認した上で engine スキャンを実行、
// findings を保存して scan を done にする。設定不備（不正 URL・所有未確認）は再試行しても
// 直らないため failed にして job を完了扱いにする。engine の実行エラーは再試行に委ねる。
func (w *Worker) runScan(ctx context.Context, scanID uuid.UUID, pgID pgtype.UUID) error {
	target, err := w.queries.GetScanTarget(ctx, pgID)
	if err != nil {
		return fmt.Errorf("get scan target %s: %w", scanID, err)
	}

	scope, err := engine.NewScope(target.BaseUrl)
	if err != nil {
		w.logger.Error("invalid scan target; marking failed", "scan_id", scanID, "err", err)
		w.markFailed(ctx, scanID, pgID)
		return nil
	}

	// defense-in-depth（ADR-0004）: api の受付ゲートに加え worker でも所有確認する。
	// localhost / 127.0.0.1 / ::1 / *.local はスキップ。未確認なら実行しない。
	if scope.RequiresOwnershipVerification() && !target.OwnershipVerified {
		w.logger.Warn("scan target ownership not verified; marking failed",
			"scan_id", scanID, "host", scope.Host())
		w.markFailed(ctx, scanID, pgID)
		return nil
	}

	findings, err := w.executeScan(ctx, pgID, scope)
	if err != nil {
		// 一過性の可能性があるため failed にはせず、river の再試行に委ねる。
		return fmt.Errorf("scan %s: %w", scanID, err)
	}

	payload, err := json.Marshal(scanSummary{Findings: engine.Summarize(findings)})
	if err != nil {
		return fmt.Errorf("marshal summary %s: %w", scanID, err)
	}

	// running -> done。既に done/failed 等で running でなければ冪等に成功扱いにする。
	if _, err := w.queries.CompleteScan(ctx, db.CompleteScanParams{ID: pgID, SummaryJson: payload}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.logger.Info("scan no longer running at completion; treating as done", "scan_id", scanID)
			return nil
		}
		return fmt.Errorf("complete scan %s: %w", scanID, err)
	}
	w.logger.Info("scan completed", "scan_id", scanID, "findings", len(findings))
	return nil
}

// executeScan は engine スキャンを実行し、検出ごとに finding を保存しつつ収集して返す。
// onFinding は複数 goroutine から呼ばれ得るため mutex で直列化する（contract 準拠）。
func (w *Worker) executeScan(ctx context.Context, pgID pgtype.UUID, scope engine.Scope) ([]engine.Finding, error) {
	var (
		mu        sync.Mutex
		collected []engine.Finding
		saveErr   error
	)
	onFinding := func(f engine.Finding) {
		mu.Lock()
		defer mu.Unlock()
		if saveErr != nil {
			return
		}
		if _, err := w.queries.InsertFinding(ctx, insertParams(pgID, f)); err != nil {
			saveErr = fmt.Errorf("insert finding %s: %w", f.TemplateID, err)
			return
		}
		collected = append(collected, f)
	}

	if err := w.engine.Scan(ctx, engine.ScanRequest{Scope: scope}, onFinding); err != nil {
		return nil, err
	}
	if saveErr != nil {
		return nil, saveErr
	}
	return collected, nil
}

// markFailed は scan を failed にする。既に終端状態（ErrNoRows）なら何もしない。
func (w *Worker) markFailed(ctx context.Context, scanID uuid.UUID, pgID pgtype.UUID) {
	if _, err := w.queries.FailScan(ctx, pgID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		w.logger.Error("mark scan failed", "scan_id", scanID, "err", err)
	}
}

// insertParams は engine.Finding を InsertFinding の引数に変換する。
func insertParams(pgID pgtype.UUID, f engine.Finding) db.InsertFindingParams {
	return db.InsertFindingParams{
		ScanID:      pgID,
		TemplateID:  f.TemplateID,
		Title:       f.Title,
		Severity:    string(f.Severity),
		Url:         f.URL,
		Cwe:         textOrNull(f.CWE),
		Remediation: textOrNull(f.Remediation),
	}
}

// textOrNull は空文字を SQL NULL に、非空を有効値にマップする。
func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
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
