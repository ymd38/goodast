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

	nucleilib "github.com/projectdiscovery/nuclei/v3/lib"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"
	nucleitypes "github.com/projectdiscovery/nuclei/v3/pkg/types"

	"github.com/ymd38/goodast/worker/internal/engine"
)

// version は go.mod で固定している Nuclei SDK のバージョン（ADR-0002）。
// go.mod の require を更新したらここも同期させる（scans.engine_version に記録される）。
const version = "nuclei/v3.9.0"

// Engine は engine.Engine の Nuclei 実装。実行パラメータは per-scan で ScanRequest.Profile
// から受け取るため、Engine 自体は状態を持たない（プリセット差はリクエスト側で表現する）。
type Engine struct{}

// New は Nuclei エンジンを生成する。
func New() *Engine { return &Engine{} }

// Version は固定された Nuclei SDK バージョン識別子を返す。
func (e *Engine) Version() string { return version }

// Scan は対象スコープに対し Nuclei スキャンを 1 回実行する。
//
// 並行スキャン（river MaxWorkers > 1）でも状態を共有しないよう、呼び出しごとに
// NucleiEngine を生成・破棄する。検出は engine 層ガードレール（スコープ allowlist /
// 危険パス除外）を通過したものだけ onFinding へ渡す。
//
// ガードレールの強制レベル（既知の制約・要レビュー）:
//   - テンプレート選択: 破壊的タグ（dos / intrusive）を ExcludeTags で除外（リクエスト前に効く）。
//   - レート: WithGlobalRateLimit で保守的に抑制。
//   - 検出フィルタ: toFinding で scope.Allows（同一 host:port・非危険パス）を満たさない結果を破棄。
//
// 注意（持ち越し）: Nuclei lib には per-request の host/path allowlist を安全に注入する手段が
// ない（WithOptions は opts を丸ごと置換し既定を壊す）。現状は単一ターゲット・非クロール
// （katana 無効）・DAST fuzzing 無効のため逸脱の主経路はテンプレートの固定パス要求と
// クロスホスト redirect に限られる。リクエスト時の厳密な host/path 遮断（カスタム transport /
// redirect ポリシー）はクロール・認証スキャン導入フェーズで実装する。
func (e *Engine) Scan(ctx context.Context, req engine.ScanRequest, onFinding engine.FindingCallback) error {
	opts := []nucleilib.NucleiSDKOptions{
		nucleilib.WithTemplateFilters(nucleilib.TemplateFilters{
			Severity:    req.Profile.Severities,
			Tags:        req.Profile.Tags,
			ExcludeTags: req.Profile.ExcludeTags,
		}),
		// 保守的な全体レート制限（DoS 化防止）。
		nucleilib.WithGlobalRateLimit(req.Profile.RateLimit, req.Profile.RatePeriod),
		// テンプレート由来のローカルファイル読取を禁止（過去の LFI/RCE テンプレートリスク対策）。
		// ローカルネットワーク制限はしない（localhost 等の自前ターゲットを許可するため）。
		nucleilib.WithSandboxOptions(false, false),
		// テンプレートのバージョンは運用で固定する。実行時の自動更新チェックは無効化する。
		nucleilib.DisableUpdateCheck(),
	}
	// 認証後スキャン: 持ち込みセッションを全リクエストに注入する（ADR-0003）。値はログしない。
	//
	// W3 対策（クロスホスト redirect での認証ヘッダ漏えい防止）: WithHeaders は SDK の全リクエストに
	// ヘッダを付与するため、テンプレートが redirects を有効化しクロスホスト redirect が起きると
	// Cookie/Bearer が意図しないホストへ送られ得る。認証情報を注入するときに限り DisableRedirects で
	// redirect 追従を止め、認証情報が別ホストへ送出される経路を塞ぐ。
	//   - DisableRedirects は httpclientpool 側でテンプレートの redirects:true すら上書きし、
	//     全 redirect を DontFollowRedirect（http.ErrUseLastResponse）に強制する（SDK v3.9.0 で確認）。
	//   - WithOptions は e.opts を全置換するため、既定（types.DefaultOptions）から作った base を
	//     先頭に適用し、後続の WithTemplateFilters / WithGlobalRateLimit 等をその上に重ねる。
	//   - 未認証スキャンでは適用しない（漏らす秘密が無く、redirect 追従は検知に有用・§10 parity 無影響）。
	if len(req.Headers) > 0 {
		base := nucleitypes.DefaultOptions()
		base.DisableRedirects = true
		opts = append([]nucleilib.NucleiSDKOptions{nucleilib.WithOptions(base)}, opts...)
		opts = append(opts, nucleilib.WithHeaders(req.Headers))
	}

	ne, err := nucleilib.NewNucleiEngineCtx(ctx, opts...)
	if err != nil {
		return fmt.Errorf("create nuclei engine: %w", err)
	}
	defer ne.Close()

	ne.LoadTargets(engine.TargetsOrBase(req), false)

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
