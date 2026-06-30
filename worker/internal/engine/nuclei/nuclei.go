// Package nuclei は engine.Engine の Nuclei v3 SDK 実装。
//
// ADR-0002: Nuclei SDK の import はこのパッケージにのみ許可する。SDK 呼び出しは
// ネットワーク・テンプレートを要しユニットテストできないため、本パッケージは薄いグルーに
// 留め、検証は //go:build integration のテストで行う（coverage の unit 計測からは除外）。
//
// 純粋ロジック（スコープ allowlist・危険パス除外・severity 正規化・集計）は親パッケージ
// engine に置き、ユニットテストで網羅している。本パッケージはそれらを呼び出すだけにする。
package nuclei

import (
	"context"
	"fmt"
	"sync"
	"time"

	nucleilib "github.com/projectdiscovery/nuclei/v3/lib"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"

	"github.com/ymd38/goodast/worker/internal/engine"
)

// Config は Nuclei エンジンの保守的な実行設定（ADR の保守的レート方針）。
type Config struct {
	// RateLimit は RatePeriod あたりの最大リクエスト数。
	RateLimit int
	// RatePeriod はレート制限の単位時間。
	RatePeriod time.Duration
	// Severities は実行対象 severity の CSV（空文字なら全 severity）。
	Severities string
	// ExcludeTags は除外タグ。破壊的テンプレート（dos 等）をデフォルト無効化する。
	ExcludeTags []string
}

// DefaultConfig は PoC の保守的デフォルト設定を返す。
func DefaultConfig() Config {
	return Config{
		RateLimit:   10,
		RatePeriod:  time.Second,
		Severities:  "",
		ExcludeTags: []string{"dos"},
	}
}

// version は go.mod で固定している Nuclei SDK のバージョン（ADR-0002）。
// go.mod の require を更新したらここも同期させる（scans.engine_version に記録される）。
const version = "nuclei/v3.9.0"

// Engine は engine.Engine の Nuclei 実装。
type Engine struct {
	cfg Config
}

// New は Nuclei エンジンを生成する。
func New(cfg Config) *Engine {
	return &Engine{cfg: cfg}
}

// Version は固定された Nuclei SDK バージョン識別子を返す。
func (e *Engine) Version() string { return version }

// Scan は対象スコープに対し Nuclei スキャンを 1 回実行する。
//
// 並行スキャン（river MaxWorkers > 1）でも状態を共有しないよう、呼び出しごとに
// NucleiEngine を生成・破棄する。検出は engine 層ガードレール（スコープ allowlist /
// 危険パス除外）を通過したものだけ onFinding へ渡す。
func (e *Engine) Scan(ctx context.Context, req engine.ScanRequest, onFinding engine.FindingCallback) error {
	ne, err := nucleilib.NewNucleiEngineCtx(ctx,
		nucleilib.WithTemplateFilters(nucleilib.TemplateFilters{
			Severity:    e.cfg.Severities,
			ExcludeTags: e.cfg.ExcludeTags,
		}),
		// 保守的な全体レート制限（DoS 化防止）。
		nucleilib.WithGlobalRateLimit(e.cfg.RateLimit, e.cfg.RatePeriod),
		// テンプレート由来のローカルファイル読取を禁止（過去の LFI/RCE テンプレートリスク対策）。
		// ローカルネットワーク制限はしない（localhost 等の自前ターゲットを許可するため）。
		nucleilib.WithSandboxOptions(false, false),
		// テンプレートのバージョンは運用で固定する。実行時の自動更新チェックは無効化する。
		nucleilib.DisableUpdateCheck(),
	)
	if err != nil {
		return fmt.Errorf("create nuclei engine: %w", err)
	}
	defer ne.Close()

	ne.LoadTargets([]string{req.Scope.BaseURL()}, false)

	// 検出コールバックは複数 goroutine から呼ばれ得るため mutex で直列化する。
	var mu sync.Mutex
	cb := func(ev *output.ResultEvent) {
		f, ok := toFinding(ev, req.Scope)
		if !ok {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		onFinding(f)
	}

	if err := ne.ExecuteCallbackWithCtx(ctx, cb); err != nil {
		return fmt.Errorf("execute nuclei scan: %w", err)
	}
	return nil
}

// toFinding は Nuclei の ResultEvent を engine.Finding に変換する。
// スコープ外・危険パスの検出は破棄する（engine 層ガードレールの最終適用点）。
func toFinding(ev *output.ResultEvent, scope engine.Scope) (engine.Finding, bool) {
	if ev == nil {
		return engine.Finding{}, false
	}
	loc := findingURL(ev)
	if !scope.Allows(loc) {
		return engine.Finding{}, false
	}
	return engine.Finding{
		TemplateID:  ev.TemplateID,
		Title:       ev.Info.Name,
		Severity:    engine.ParseSeverity(ev.Info.SeverityHolder.Severity.String()),
		URL:         loc,
		CWE:         cweOf(ev),
		Remediation: ev.Info.Remediation,
	}, true
}

// findingURL は検出箇所 URL を matched-at > url > host の優先で返す。
func findingURL(ev *output.ResultEvent) string {
	if ev.Matched != "" {
		return ev.Matched
	}
	if ev.URL != "" {
		return ev.URL
	}
	return ev.Host
}

// cweOf はテンプレートの CWE 分類を返す（未設定なら空文字）。
func cweOf(ev *output.ResultEvent) string {
	if ev.Info.Classification == nil {
		return ""
	}
	return ev.Info.Classification.CWEID.String()
}

// 静的アサーション: Engine が engine.Engine を満たすことを保証する。
var _ engine.Engine = (*Engine)(nil)
