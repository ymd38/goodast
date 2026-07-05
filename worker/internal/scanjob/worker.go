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
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/jobs"
	"github.com/ymd38/goodast/secrets"
	"github.com/ymd38/goodast/worker/internal/db"
	"github.com/ymd38/goodast/worker/internal/engine"
)

// Worker は scan ジョブの river.Worker 実装。
type Worker struct {
	river.WorkerDefaults[jobs.ScanArgs]
	queries *db.Queries
	engine  engine.Engine
	cipher  *secrets.Cipher
	logger  *slog.Logger
}

// WorkerDeps は Worker の依存（dig struct-based injection）。
type WorkerDeps struct {
	dig.In
	Queries *db.Queries
	Engine  engine.Engine
	Cipher  *secrets.Cipher
	Logger  *slog.Logger
}

// NewWorker は scan ジョブワーカーを生成する。
func NewWorker(d WorkerDeps) *Worker {
	return &Worker{queries: d.Queries, engine: d.Engine, cipher: d.Cipher, logger: d.Logger}
}

// Timeout は scan ジョブ1回あたりの実行上限。river 既定の1分では nuclei スキャンが
// 完走できず context deadline exceeded を繰り返すため、暫定で余裕を持たせる
// （正式なタイムアウト設計・プリセット別の値設定は別タスク）。
func (w *Worker) Timeout(*river.Job[jobs.ScanArgs]) time.Duration {
	return 10 * time.Minute
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

	// 最終試行かを判定する。engine の一過性エラーは再試行に委ね、最後の試行でも
	// 失敗した場合は failed に確定させて scan が running のまま残らないようにする（#4）。
	lastAttempt := job.Attempt >= job.MaxAttempts
	return w.runScan(ctx, scanID, pgID, lastAttempt)
}

// runScan はスキャン対象をロードし、ガードレールを確認した上で engine スキャンを実行、
// findings を保存して scan を done にする。設定不備（不正 URL・所有未確認）は再試行しても
// 直らないため failed にする。engine の実行エラーは原則再試行に委ね、最終試行で失敗した
// 場合のみ failed に確定する。状態更新（FailScan）に失敗した場合は error を返し、
// scan が running のまま握り潰されないようにする（#7）。
func (w *Worker) runScan(ctx context.Context, scanID uuid.UUID, pgID pgtype.UUID, lastAttempt bool) error {
	target, err := w.queries.GetScanTarget(ctx, pgID)
	if err != nil {
		return fmt.Errorf("get scan target %s: %w", scanID, err)
	}

	scope, err := engine.NewScope(target.BaseUrl)
	if err != nil {
		w.logger.Error("invalid scan target; marking failed", "scan_id", scanID, "err", err)
		return w.markFailed(ctx, scanID, pgID)
	}

	// defense-in-depth（ADR-0004）: api の受付ゲートに加え worker でも所有確認する。
	// localhost / 127.0.0.1 / ::1 / *.local はスキップ。未確認なら実行しない。
	if scope.RequiresOwnershipVerification() && !target.OwnershipVerified {
		w.logger.Warn("scan target ownership not verified; marking failed",
			"scan_id", scanID, "host", scope.Host())
		return w.markFailed(ctx, scanID, pgID)
	}

	// 実行前に必ず既存 findings を掃除する（再試行・途中失敗でも stale findings を残さない・#5）。
	// 認証情報ロードより前に行い、復号失敗で failed 確定する場合も古い結果が残らないようにする。
	if err := w.queries.DeleteFindingsByScan(ctx, pgID); err != nil {
		return fmt.Errorf("clear prior findings %s: %w", scanID, err)
	}

	// 認証情報（持ち込みセッション）をロード・復号する（ADR-0003）。ヘッダ値は一切ログしない。
	headers, err := w.loadHeaders(ctx, pgID, uuid.UUID(target.SiteID.Bytes))
	if err != nil {
		// 復号/検証失敗は設定・データ不整合で再試行しても直らないため failed に確定する。
		if permanentCredentialError(err) {
			w.logger.Error("credential decrypt/validation failed; marking failed", "scan_id", scanID, "err", err)
			return w.markFailed(ctx, scanID, pgID)
		}
		// DB 一時障害等は engine 実行エラーと同様に river の再試行へ委ね、最終試行のみ failed 確定。
		if lastAttempt {
			w.logger.Error("credential load failed on final attempt; marking failed", "scan_id", scanID, "err", err)
			return w.markFailed(ctx, scanID, pgID)
		}
		return fmt.Errorf("load credentials %s: %w", scanID, err)
	}
	if len(headers) > 0 {
		w.logger.Info("authenticated scan; injecting session headers", "scan_id", scanID, "header_count", len(headers))
	}

	findings, err := w.executeScan(ctx, pgID, scope, headers)
	if err != nil {
		if lastAttempt {
			// 最終試行でも失敗: failed に確定する。状態更新に失敗したら error を返し再試行に回す。
			w.logger.Error("scan failed on final attempt; marking failed", "scan_id", scanID, "err", err)
			return w.markFailed(ctx, scanID, pgID)
		}
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
// 既存 findings の掃除（再実行の冪等化・#5）は呼び出し元 runScan が実行前に行う。
func (w *Worker) executeScan(ctx context.Context, pgID pgtype.UUID, scope engine.Scope, headers []string) ([]engine.Finding, error) {
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

	if err := w.engine.Scan(ctx, engine.ScanRequest{Scope: scope, Headers: headers}, onFinding); err != nil {
		return nil, err
	}
	if saveErr != nil {
		return nil, saveErr
	}
	return collected, nil
}

// loadHeaders は当該 scan の持ち込みセッションをロード・復号し nuclei 形式のヘッダに変換する。
// 認証情報が無い（未設定）scan は nil を返す（未認証スキャン）。復号 AAD は site_id。
// 復号済みヘッダの値は呼び出し側含め一切ログしない（ADR-0003）。
func (w *Worker) loadHeaders(ctx context.Context, pgID pgtype.UUID, siteID uuid.UUID) ([]string, error) {
	row, err := w.queries.GetScanCredentials(ctx, pgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // 認証情報なし = 未認証スキャン。
	}
	if err != nil {
		return nil, fmt.Errorf("get scan credentials: %w", err)
	}
	// 「none は行の不在」が原則だが、残存する none 行は未認証として扱う。
	if row.AuthMode != "session" {
		return nil, nil
	}
	headers, err := w.cipher.OpenHeaders(secrets.EncryptedHeaders(row.EncHeaders), siteID[:])
	if err != nil {
		return nil, fmt.Errorf("decrypt credentials: %w", err)
	}
	return headers.ToNucleiFormat(), nil
}

// permanentCredentialError は再試行で解消しない認証情報エラー（復号失敗・不正ヘッダ）かを判定する。
// これらは鍵/データ不整合が原因で、DB 一時障害のような一過性エラーとは区別して即 failed にする。
func permanentCredentialError(err error) bool {
	return errors.Is(err, secrets.ErrDecrypt) ||
		errors.Is(err, secrets.ErrInvalidHeader) ||
		errors.Is(err, secrets.ErrNoHeaders)
}

// markFailed は scan を failed にし、ジョブを完了扱い（nil）にしてよいかを返す。
// 既に終端状態（ErrNoRows）なら冪等に nil を返す。状態更新そのものに失敗した場合は
// error を返し、scan が running のまま握り潰されないようにする（#7）。
func (w *Worker) markFailed(ctx context.Context, scanID uuid.UUID, pgID pgtype.UUID) error {
	if _, err := w.queries.FailScan(ctx, pgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // 既に終端状態。失敗確定済みとして扱う。
		}
		return fmt.Errorf("mark scan %s failed: %w", scanID, err)
	}
	return nil
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
