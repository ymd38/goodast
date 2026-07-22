// Package katana は engine.Crawler の Katana v1.6.1 SDK 実装。
//
// ADR-0002 に準じ Katana SDK の import はこのパッケージにのみ許可する。SDK 呼び出しは
// ネットワーク＋クロール対象を要しユニットテストできないため、本パッケージは薄いグルーに留め、
// 検証は //go:build integration のテストで行う（coverage の unit 計測からは除外）。
//
// クロール段ガード（spec §3）: standard engine（GET-only・フォーム送信なし）、FieldScope=fqdn で
// seed ホストに限定、危険パスを OutOfScope で辿らせない、OnResult で scope.Allows 後段チェック、
// MaxURLs 到達／ctx キャンセルで crawler.Close() して停止する。
//
// 注（Katana v1.6.1・Task 6 の go doc で確認済み）: types.Options に Context フィールドは無い。
// Crawl(rootURL) は blocking なので、goroutine で回して ctx.Done() / MaxURLs 到達時に Close() で止める。
// standard.Crawler は Close() error を持つ。OnResultCallback は func(output.Result)。
// output.Result.Request は *navigation.Request（Method string / URL string）。
//
// 注2（integration 実走で判明）: types.Options.Strategy を明示しないと内部の
// pkg/utils/queue.New が "unsupported strategy" で失敗する（ゼロ値の "" は未定義戦略）。
// katana CLI の既定 "depth-first" を明示指定する。
package katana

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/projectdiscovery/katana/pkg/engine/standard"
	"github.com/projectdiscovery/katana/pkg/output"
	"github.com/projectdiscovery/katana/pkg/types"

	"github.com/ymd38/goodast/worker/internal/engine"
)

// version は go.mod で固定している Katana SDK バージョン（ADR-0002）。
const version = "katana/v1.6.1"

// Crawler は engine.Crawler の Katana 実装。状態を持たず per-scan で使える。
type Crawler struct{}

// New は Katana クローラを生成する。
func New() *Crawler { return &Crawler{} }

// Version は固定された Katana SDK バージョン識別子を返す。
func (c *Crawler) Version() string { return version }

// Crawl は scope 起点から GET 探索し、スコープ内の GET 到達 URL と抽出フォーム数を返す。
func (c *Crawler) Crawl(ctx context.Context, scope engine.Scope, plan engine.CrawlPlan, headers []string) (engine.CrawlResult, error) {
	var (
		mu        sync.Mutex
		seen      = make(map[string]struct{})
		urls      []string
		forms     int
		cr        *standard.Crawler
		closeOnce sync.Once
		stopped   atomic.Bool
	)
	// stop は crawler を一度だけ Close し、停止フラグを立てる。MaxURLs 到達・ctx キャンセル・
	// 正常終了のいずれからも安全に呼べる（Close は OnResult ワーカ外の goroutine から呼ぶ）。
	stop := func() {
		stopped.Store(true)
		closeOnce.Do(func() {
			if cr != nil {
				_ = cr.Close()
			}
		})
	}

	opts := &types.Options{
		URLs:              []string{scope.BaseURL()},
		MaxDepth:          plan.MaxDepth,
		Strategy:          "depth-first", // katana CLI 既定。空文字だと queue.New が "unsupported strategy" で失敗する
		FieldScope:        "fqdn",        // seed の完全一致ホストに限定（サブドメイン追わない）
		OutOfScope:        engine.DangerousPathRegexes(),
		FormExtraction:    true,  // フォームを抽出（送信はしない）
		AutomaticFormFill: false, // 明示的に無効化: フォーム送信を絶対にしない（GET-only 保証・上流既定が変わっても安全側）
		CustomHeaders:     headers,
		OnResult: func(r output.Result) {
			if r.Request == nil {
				return
			}
			method := strings.ToUpper(r.Request.Method)
			u := r.Request.URL
			mu.Lock()
			defer mu.Unlock()
			// 非 GET（フォームのアクション等）は診断対象 URL に入れず件数のみ集計する。
			// HEAD は GET と同じく副作用が無く診断対象になり得るため GET 相当に扱う。
			if method != "" && method != "GET" && method != "HEAD" {
				forms++
				return
			}
			// host:port＋非危険パスの後段チェック（belt-and-suspenders）。
			if !scope.Allows(u) {
				return
			}
			if _, dup := seen[u]; dup {
				return
			}
			// MaxURLs はハード上限: 到達後は append せず、未停止なら Close を仕掛ける。
			// append 前に判定することで in-flight 結果による上限超過（オーバーシュート）を防ぐ。
			if plan.MaxURLs > 0 && len(urls) >= plan.MaxURLs {
				if !stopped.Load() {
					go stop() // Close は OnResult ワーカ外の goroutine から
				}
				return
			}
			seen[u] = struct{}{}
			urls = append(urls, u)
		},
	}

	crawlerOpts, err := types.NewCrawlerOptions(opts)
	if err != nil {
		return engine.CrawlResult{}, fmt.Errorf("katana options: %w", err)
	}
	defer crawlerOpts.Close()

	cr, err = standard.New(crawlerOpts)
	if err != nil {
		return engine.CrawlResult{}, fmt.Errorf("katana engine: %w", err)
	}
	defer stop() // 正常終了でも確実に Close（sync.Once で二重 Close を防ぐ）

	// Crawl は blocking。goroutine で回し、ctx キャンセル時は Close で止める。
	done := make(chan error, 1)
	go func() { done <- cr.Crawl(scope.BaseURL()) }()

	select {
	case <-ctx.Done():
		stop()
		<-done // クロール goroutine の終了を待つ
	case cerr := <-done:
		// 自発 Close（MaxURLs / ctx）由来のエラーは正常終了扱い。それ以外は失敗。
		if cerr != nil && !stopped.Load() {
			return engine.CrawlResult{}, fmt.Errorf("katana crawl: %w", cerr)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	return engine.CrawlResult{URLs: urls, FormCount: forms}, nil
}

// 静的アサーション: Crawler が engine.Crawler を満たすことを保証する。
var _ engine.Crawler = (*Crawler)(nil)
