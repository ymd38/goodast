# Katana 探索（Discovery）機能 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Katana Go SDK を独立 `Crawler` として worker に組み込み、GET 探索で発見した URL 群を既存 Nuclei 診断に流して診断対象を単一 URL から発見 URL 群へ拡張する（探索コアのみ・フォームファジングは対象外）。

**Architecture:** 純粋層 `engine` に `Crawler` interface・`CrawlPlan`・動的タイムアウト式・`TargetsOrBase` を足し、Katana SDK 実装は `engine/discovery/katana` に隔離する（既存 `engine/nuclei` と対称）。`scanjob/worker.go` を「crawl段 → 動的タイムアウト算出 → scan段」の二段に配線する。クロール段のスコープ/危険パス遡断は Katana native scope 設定＋ `OnResult` の `scope.Allows` 後段チェックで実現する（Katana は custom transport を受けないため）。

**Tech Stack:** Go 1.26.4 / river / dig / `github.com/projectdiscovery/katana`（Go SDK・standard engine）/ testify / slog

## Global Constraints

- Nuclei / Katana SDK の import は `worker/internal/engine/` 配下のみ（ADR-0001 / ADR-0002）。api からは import しない。
- Katana バージョンは `go.mod` で固定し `go get -u` しない（ADR-0002 準拠）。CLI バイナリは同梱しない（SDK 静的リンク・【決定 2026-07-02】）。
- スコープは登録ドメインの allowlist に限定。危険パス（`logout`/`signout`/`delete`/`remove`/`destroy`/`admin`）はデフォルト除外。`engine.Scope` を allowlist・危険パスの唯一の正とする。
- クロール段は GET のみ・フォーム送信しない（standard engine・`AutomaticFormFill` 無効）。
- 認証情報・ヘッダ値は平文ログ禁止（ADR-0003）。
- 構造化ログ（slog）。`fmt.Println`/`log.Printf` 禁止。エラーは `fmt.Errorf("...: %w", err)` でラップ。
- テスト: テーブル駆動・`-race` 必須。純粋層は C0 100%。SDK を呼ぶテストは `//go:build integration` で分離しユニット coverage から除外。
- 生成 sqlc コード（`internal/db/*.go`）は編集しない。本タスクは DB マイグレーション不要（`discovered_endpoints` テーブルは次タスク）。

参照 spec: `docs/superpowers/specs/2026-07-23-katana-discovery-design.md`

---

### Task 1: `engine.CrawlPlan` と `PlanFor` のクロール上限

**Files:**
- Create: `worker/internal/engine/crawl.go`
- Modify: `worker/internal/engine/preset.go`（`Plan` に `Crawl` 追加・`plan()`・`PlanFor()`）
- Test: `worker/internal/engine/preset_test.go`（追記）

**Interfaces:**
- Produces: `engine.CrawlPlan{Enabled bool; MaxDepth int; MaxURLs int}`、`engine.Plan.Crawl CrawlPlan`。

- [ ] **Step 1: 失敗するテストを書く**

`worker/internal/engine/preset_test.go` に追記:

```go
func TestPlanForCrawlBounds(t *testing.T) {
	tests := []struct {
		name    string
		preset  jobs.Preset
		enabled bool
		depth   int
		maxURLs int
	}{
		{"light はクロール無効", jobs.PresetLight, false, 0, 0},
		{"standard は浅いクロール", jobs.PresetStandard, true, 2, 50},
		{"deep は広いクロール", jobs.PresetDeep, true, 3, 200},
		{"未知は standard 既定", jobs.Preset("bogus"), true, 2, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanFor(tt.preset).Crawl
			if got.Enabled != tt.enabled || got.MaxDepth != tt.depth || got.MaxURLs != tt.maxURLs {
				t.Fatalf("Crawl = %+v; want {Enabled:%v MaxDepth:%d MaxURLs:%d}",
					got, tt.enabled, tt.depth, tt.maxURLs)
			}
		})
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd worker && go test ./internal/engine/ -run TestPlanForCrawlBounds`
Expected: FAIL（`Crawl` フィールド未定義でコンパイルエラー）

- [ ] **Step 3: 最小実装**

`worker/internal/engine/crawl.go` を新規作成:

```go
package engine

// CrawlPlan はクロール段の有界化パラメータ。preset から導出される。
// Enabled=false のとき crawler は呼ばれず、診断は単一 URL のまま（現状維持）。
type CrawlPlan struct {
	Enabled  bool
	MaxDepth int
	MaxURLs  int
}
```

`worker/internal/engine/preset.go` の `Plan` に `Crawl` を追加:

```go
type Plan struct {
	Scan    ScanProfile
	Timeout time.Duration
	Crawl   CrawlPlan
}
```

`plan()` ヘルパに crawl 引数を足す:

```go
func plan(tags []string, timeout time.Duration, crawl CrawlPlan) Plan {
	return Plan{
		Scan: ScanProfile{
			Tags:        tags,
			ExcludeTags: excludeDestructive,
			Severities:  "",
			RateLimit:   10,
			RatePeriod:  time.Second,
		},
		Timeout: timeout,
		Crawl:   crawl,
	}
}
```

`PlanFor()` の各 case に crawl を渡す:

```go
func PlanFor(p jobs.Preset) Plan {
	switch p {
	case jobs.PresetLight:
		return plan([]string{"misconfig", "tech", "exposure"}, 15*time.Minute,
			CrawlPlan{Enabled: false})
	case jobs.PresetDeep:
		return plan([]string{
			"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve",
			"xss", "sqli", "lfi", "ssrf", "rce", "takeover",
		}, 60*time.Minute, CrawlPlan{Enabled: true, MaxDepth: 3, MaxURLs: 200})
	default: // standard を既定に
		return plan([]string{
			"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve",
		}, 30*time.Minute, CrawlPlan{Enabled: true, MaxDepth: 2, MaxURLs: 50})
	}
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `cd worker && go test ./internal/engine/ -run TestPlanForCrawlBounds -race`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add worker/internal/engine/crawl.go worker/internal/engine/preset.go worker/internal/engine/preset_test.go
git commit -m "feat(engine): preset ごとのクロール上限（CrawlPlan）を追加する"
```

---

### Task 2: `engine.ScanTimeout`（動的タイムアウト式）

**Files:**
- Modify: `worker/internal/engine/crawl.go`
- Test: `worker/internal/engine/crawl_test.go`（新規）

**Interfaces:**
- Consumes: —
- Produces: `func engine.ScanTimeout(numURLs int, ceiling time.Duration) time.Duration`。

- [ ] **Step 1: 失敗するテストを書く**

`worker/internal/engine/crawl_test.go` を新規作成:

```go
package engine

import (
	"testing"
	"time"
)

func TestScanTimeout(t *testing.T) {
	const ceiling = 30 * time.Minute
	tests := []struct {
		name    string
		numURLs int
		ceiling time.Duration
		want    time.Duration
	}{
		{"URL0 は floor", 0, ceiling, 2 * time.Minute},                 // base=2m, floor=2m
		{"少数 URL は base+加算", 6, ceiling, 3 * time.Minute},          // 2m + 6*10s = 3m
		{"多数 URL は ceiling で頭打ち", 1000, ceiling, 30 * time.Minute},
		{"ceiling が floor 未満でも ceiling を超えない", 5, 1 * time.Minute, 1 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScanTimeout(tt.numURLs, tt.ceiling); got != tt.want {
				t.Fatalf("ScanTimeout(%d, %v) = %v; want %v", tt.numURLs, tt.ceiling, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd worker && go test ./internal/engine/ -run TestScanTimeout`
Expected: FAIL（`ScanTimeout` 未定義）

- [ ] **Step 3: 最小実装**

`worker/internal/engine/crawl.go` に追記（`import "time"` を追加）:

```go
import "time"

// 動的タイムアウトの係数（spec §6.1・実測でチューニング可能）。
const (
	scanTimeoutBase    = 2 * time.Minute  // 単一 URL でも確保する下駄
	scanTimeoutPerURL  = 10 * time.Second // 発見 URL 1 本あたりの追加枠
	scanTimeoutFloor   = 2 * time.Minute
)

// ScanTimeout は発見 URL 数から scan 段の実行枠を算出する。
// base + numURLs×perURL を [floor, ceiling] にクランプする。ceiling は preset 別の絶対上限
// （= PlanFor(preset).Timeout）で、river ジョブ Timeout と一致させる（spec §6.1）。
func ScanTimeout(numURLs int, ceiling time.Duration) time.Duration {
	d := scanTimeoutBase + time.Duration(numURLs)*scanTimeoutPerURL
	if d < scanTimeoutFloor {
		d = scanTimeoutFloor
	}
	if d > ceiling {
		d = ceiling
	}
	return d
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `cd worker && go test ./internal/engine/ -run TestScanTimeout -race`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add worker/internal/engine/crawl.go worker/internal/engine/crawl_test.go
git commit -m "feat(engine): 発見 URL 数に応じた動的タイムアウト ScanTimeout を追加する"
```

---

### Task 3: `ScanRequest.Targets` と `TargetsOrBase`（Nuclei へ発見 URL を渡す配管）

**Files:**
- Modify: `worker/internal/engine/engine.go`（`ScanRequest` に `Targets`）
- Modify: `worker/internal/engine/crawl.go`（`TargetsOrBase`）
- Modify: `worker/internal/engine/nuclei/nuclei.go`（92行目 `LoadTargets` の入力）
- Test: `worker/internal/engine/crawl_test.go`（追記）

**Interfaces:**
- Consumes: `engine.Scope.BaseURL()`。
- Produces: `engine.ScanRequest.Targets []string`、`func engine.TargetsOrBase(req ScanRequest) []string`。

- [ ] **Step 1: 失敗するテストを書く**

`worker/internal/engine/crawl_test.go` に追記:

```go
func TestTargetsOrBase(t *testing.T) {
	scope, err := NewScope("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("Targets 空なら base URL", func(t *testing.T) {
		got := TargetsOrBase(ScanRequest{Scope: scope})
		if len(got) != 1 || got[0] != "https://example.com" {
			t.Fatalf("got %v; want [https://example.com]", got)
		}
	})
	t.Run("Targets 非空ならそのまま", func(t *testing.T) {
		in := []string{"https://example.com/a", "https://example.com/b"}
		got := TargetsOrBase(ScanRequest{Scope: scope, Targets: in})
		if len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
			t.Fatalf("got %v; want %v", got, in)
		}
	})
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd worker && go test ./internal/engine/ -run TestTargetsOrBase`
Expected: FAIL（`ScanRequest.Targets` / `TargetsOrBase` 未定義）

- [ ] **Step 3: 最小実装**

`worker/internal/engine/engine.go` の `ScanRequest` に `Targets` を追加:

```go
type ScanRequest struct {
	Scope Scope
	// Targets はクロール段が発見した診断対象 URL 群。空なら Scope.BaseURL() 単一にフォールバック。
	Targets []string
	Headers []string
	Profile ScanProfile
}
```

`worker/internal/engine/crawl.go` に追記:

```go
// TargetsOrBase は診断対象 URL を返す。クロールで発見した Targets があればそれを、
// 無ければ Scope.BaseURL() 単一を返す（未クロール時の後方互換）。
func TargetsOrBase(req ScanRequest) []string {
	if len(req.Targets) > 0 {
		return req.Targets
	}
	return []string{req.Scope.BaseURL()}
}
```

`worker/internal/engine/nuclei/nuclei.go` の 92行目を差し替え（ガード・他オプションは不変）:

```go
	ne.LoadTargets(engine.TargetsOrBase(req), false)
```

- [ ] **Step 4: テストとビルドが通ることを確認**

Run: `cd worker && go test ./internal/engine/ -run TestTargetsOrBase -race && go build ./...`
Expected: PASS かつビルド成功

- [ ] **Step 5: コミット**

```bash
git add worker/internal/engine/engine.go worker/internal/engine/crawl.go worker/internal/engine/crawl_test.go worker/internal/engine/nuclei/nuclei.go
git commit -m "feat(engine): ScanRequest.Targets を追加し nuclei に発見 URL 群を渡せるようにする"
```

---

### Task 4: `Crawler` interface・`CrawlResult`・`DangerousPathRegexes`

**Files:**
- Modify: `worker/internal/engine/engine.go`（`Crawler` / `CrawlResult`）
- Modify: `worker/internal/engine/crawl.go`（`DangerousPathRegexes`）
- Test: `worker/internal/engine/crawl_test.go`（追記）

**Interfaces:**
- Consumes: `engine.Scope`、`engine.CrawlPlan`、`dangerousPathSegments`（scope.go の非公開スライス）。
- Produces: `engine.Crawler` interface、`engine.CrawlResult{URLs []string; FormCount int}`、`func engine.DangerousPathRegexes() []string`。

- [ ] **Step 1: 失敗するテストを書く**

`worker/internal/engine/crawl_test.go` に追記:

```go
import (
	"regexp"
	// 既存の testing / time に加える
)

func TestDangerousPathRegexes(t *testing.T) {
	pats := DangerousPathRegexes()
	if len(pats) == 0 {
		t.Fatal("regex が空")
	}
	matchAny := func(path string) bool {
		for _, p := range pats {
			if regexp.MustCompile(p).MatchString(path) {
				return true
			}
		}
		return false
	}
	blocked := []string{"/logout", "/user/delete", "/admin/panel", "/account/remove", "/DESTROY"}
	for _, p := range blocked {
		if !matchAny(p) {
			t.Errorf("危険パス %q が regex に一致しない", p)
		}
	}
	allowed := []string{"/", "/products", "/search?q=1", "/administrator-guide"}
	for _, p := range allowed {
		if matchAny(p) {
			t.Errorf("非危険パス %q が誤って一致した", p)
		}
	}
}
```

> 注: `/administrator-guide` は `admin` を含むが独立セグメントではないため一致してはならない（`IsDangerousPath` と整合）。

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd worker && go test ./internal/engine/ -run TestDangerousPathRegexes`
Expected: FAIL（`DangerousPathRegexes` 未定義）

- [ ] **Step 3: 最小実装**

`worker/internal/engine/engine.go` に追記（`import "net/http"` は不要・context のみ）:

```go
// CrawlResult はクロール段の成果。URLs は GET 到達済み・スコープ内の対象（重複排除済み）。
// FormCount は抽出した非 GET フォーム（アクション）の件数（今回は件数のみ・詳細永続化は次タスク）。
type CrawlResult struct {
	URLs      []string
	FormCount int
}

// Crawler はクロールエンジンの抽象。実装は engine/discovery 配下に隔離する（Engine と同じ扱い）。
// interface の主目的はテスト容易性（scanjob を実クロールなしで検証するため fake 差し替え可能にする）。
type Crawler interface {
	// Crawl は scope 起点から plan の上限内で GET 探索し、発見 URL とフォーム数を返す。
	// headers は認証クロール用の "Name: Value"（ADR-0003・未認証時は空）。値はログしない。
	Crawl(ctx context.Context, scope Scope, plan CrawlPlan, headers []string) (CrawlResult, error)
	Version() string
}
```

`worker/internal/engine/crawl.go` に追記（`import "strings"` を追加）:

```go
// DangerousPathRegexes は危険パスセグメントを Katana の OutOfScope 正規表現として返す。
// scope.go の dangerousPathSegments を唯一の正とし、セグメント境界（/ または末尾・クエリ）で
// 区切る（`/administrator-guide` の admin 部分一致は弾く。IsDangerousPath と整合）。
func DangerousPathRegexes() []string {
	pats := make([]string, 0, len(dangerousPathSegments))
	for _, seg := range dangerousPathSegments {
		pats = append(pats, `(?i)/`+seg+`(/|$|\?)`)
	}
	return pats
}
```

- [ ] **Step 4: テストとビルドが通ることを確認**

Run: `cd worker && go test ./internal/engine/ -run TestDangerousPathRegexes -race && go build ./...`
Expected: PASS かつビルド成功

- [ ] **Step 5: コミット**

```bash
git add worker/internal/engine/engine.go worker/internal/engine/crawl.go worker/internal/engine/crawl_test.go
git commit -m "feat(engine): Crawler interface と危険パス OutOfScope regex を追加する"
```

---

### Task 5: `jobs.ScanSummary.Discovery`

**Files:**
- Modify: `jobs/summary.go`
- Test: `jobs/summary_test.go`（追記）

**Interfaces:**
- Produces: `jobs.DiscoveryInfo{URLCount int; FormCount int}`、`jobs.ScanSummary.Discovery *DiscoveryInfo`。

- [ ] **Step 1: 失敗するテストを書く**

`jobs/summary_test.go` に追記:

```go
func TestScanSummaryDiscoveryOmitempty(t *testing.T) {
	// クロール無効時は discovery キーを出さない（後方互換）。
	raw, _ := json.Marshal(ScanSummary{Findings: SeverityCounts{Total: 1}})
	if strings.Contains(string(raw), "discovery") {
		t.Fatalf("discovery が omitempty で出ていない: %s", raw)
	}
	// クロール有効時は url_count / form_count を含む。
	in := ScanSummary{
		Findings:  SeverityCounts{Total: 2},
		Discovery: &DiscoveryInfo{URLCount: 12, FormCount: 3},
	}
	raw2, _ := json.Marshal(in)
	var out ScanSummary
	if err := json.Unmarshal(raw2, &out); err != nil {
		t.Fatal(err)
	}
	if out.Discovery == nil || out.Discovery.URLCount != 12 || out.Discovery.FormCount != 3 {
		t.Fatalf("round-trip mismatch: %+v", out.Discovery)
	}
}
```

> `jobs/summary_test.go` の import に `strings` が無ければ追加する。

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd jobs && go test ./ -run TestScanSummaryDiscoveryOmitempty`
Expected: FAIL（`Discovery` / `DiscoveryInfo` 未定義）

- [ ] **Step 3: 最小実装**

`jobs/summary.go` に追記:

```go
// DiscoveryInfo はクロール段の集計（発見 URL 数・抽出フォーム数）。詳細（エンドポイント一覧）の
// 永続化は次タスク（discovered_endpoints テーブル）。
type DiscoveryInfo struct {
	URLCount  int `json:"url_count"`
	FormCount int `json:"form_count"`
}
```

`ScanSummary` に `Discovery` を追加:

```go
type ScanSummary struct {
	Findings  SeverityCounts `json:"findings"`
	Discovery *DiscoveryInfo `json:"discovery,omitempty"` // クロール無効時 nil
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `cd jobs && go test ./ -race`
Expected: PASS（既存 `TestScanSummaryRoundTrip` / `TestScanSummaryWireShapeIsNested` も継続 PASS）

- [ ] **Step 5: コミット**

```bash
git add jobs/summary.go jobs/summary_test.go
git commit -m "feat(jobs): ScanSummary にクロール集計 Discovery を追加する"
```

---

### Task 6: Katana SDK 依存の追加

**Files:**
- Modify: `worker/go.mod`, `worker/go.sum`

**Interfaces:**
- Produces: `github.com/projectdiscovery/katana`（`pkg/types` / `pkg/engine/standard` / `pkg/output`）が worker から import 可能になる。

- [ ] **Step 1: 依存を取得**

Run:
```bash
cd worker && go get github.com/projectdiscovery/katana@latest && go mod tidy
```
Expected: `go.mod` に `github.com/projectdiscovery/katana vX.Y.Z` が追加される。解決された版数を控える（次 Task の `version` 定数に使う）。

> 既存の projectdiscovery 系依存（`utils` / `retryablehttp-go` 等）と版が衝突した場合は `go mod tidy`
> のエラーに従い、Nuclei v3.9.0 と両立する katana 版へ調整する（`go get github.com/projectdiscovery/katana@vX.Y.Z`）。

- [ ] **Step 2: SDK 型の実在を確認**

Run:
```bash
cd worker && go doc github.com/projectdiscovery/katana/pkg/types Options | grep -E "MaxDepth|FieldScope|OutOfScope|FormExtraction|CustomHeaders|Context|OnResult"
cd worker && go doc github.com/projectdiscovery/katana/pkg/output Result
```
Expected: `Options` に `MaxDepth`/`FieldScope`/`OutOfScope`/`FormExtraction`/`CustomHeaders`/`Context`/`OnResult` が存在。`output.Result` が `Request`（`Method`/`URL` を持つ navigation.Request）を持つことを確認。**フィールド名が異なる場合は Task 7 の実装で実名に合わせる**（このコマンドの出力が正）。

- [ ] **Step 3: ビルド確認**

Run: `cd worker && go build ./...`
Expected: 成功（まだ katana は未使用なので indirect でも可）。

- [ ] **Step 4: コミット**

```bash
git add worker/go.mod worker/go.sum
git commit -m "chore(worker): Katana Go SDK を依存に追加する"
```

---

### Task 7: Katana アダプタ（`engine.Crawler` 実装）

**Files:**
- Create: `worker/internal/engine/discovery/katana/katana.go`
- Test: `worker/internal/engine/discovery/katana/katana_integration_test.go`

**Interfaces:**
- Consumes: `engine.Scope`（`BaseURL`/`Allows`）、`engine.CrawlPlan`、`engine.DangerousPathRegexes()`、`engine.CrawlResult`、Katana SDK。
- Produces: `katana.New() *Crawler`（`engine.Crawler` 実装）。

- [ ] **Step 1: アダプタ本体を実装**

`worker/internal/engine/discovery/katana/katana.go` を新規作成（`version` は Task 6 で控えた実際の版に置換）:

```go
// Package katana は engine.Crawler の Katana v… SDK 実装。
//
// ADR-0002 に準じ Katana SDK の import はこのパッケージにのみ許可する。SDK 呼び出しは
// ネットワーク＋クロール対象を要しユニットテストできないため、本パッケージは薄いグルーに留め、
// 検証は //go:build integration のテストで行う（coverage の unit 計測からは除外）。
//
// クロール段ガード（spec §3）: standard engine（GET-only・フォーム送信なし）、FieldScope=fqdn で
// seed ホストに限定、危険パスを OutOfScope で辿らせない、OnResult で scope.Allows 後段チェック、
// MaxURLs 到達で context を cancel して停止する。
package katana

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/projectdiscovery/katana/pkg/engine/standard"
	"github.com/projectdiscovery/katana/pkg/output"
	"github.com/projectdiscovery/katana/pkg/types"

	"github.com/ymd38/goodast/worker/internal/engine"
)

// version は go.mod で固定している Katana SDK バージョン（ADR-0002・Task 6 の実値に合わせる）。
const version = "katana/vX.Y.Z"

// Crawler は engine.Crawler の Katana 実装。状態を持たず per-scan で使える。
type Crawler struct{}

// New は Katana クローラを生成する。
func New() *Crawler { return &Crawler{} }

// Version は固定された Katana SDK バージョン識別子を返す。
func (c *Crawler) Version() string { return version }

// Crawl は scope 起点から GET 探索し、スコープ内の GET 到達 URL と抽出フォーム数を返す。
func (c *Crawler) Crawl(ctx context.Context, scope engine.Scope, plan engine.CrawlPlan, headers []string) (engine.CrawlResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu    sync.Mutex
		seen  = make(map[string]struct{})
		urls  []string
		forms int
	)

	opts := &types.Options{
		MaxDepth:       plan.MaxDepth,
		FieldScope:     "fqdn", // seed の完全一致ホストに限定（サブドメイン追わない）
		OutOfScope:     engine.DangerousPathRegexes(),
		FormExtraction: true, // フォームを抽出（送信はしない・standard は AutomaticFormFill 無効）
		CustomHeaders:  headers,
		Context:        ctx,
		OnResult: func(r output.Result) {
			if r.Request == nil {
				return
			}
			method := strings.ToUpper(r.Request.Method)
			u := r.Request.URL
			mu.Lock()
			defer mu.Unlock()
			// 非 GET（フォームのアクション等）は診断対象 URL に入れず件数のみ集計する。
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
			seen[u] = struct{}{}
			urls = append(urls, u)
			if plan.MaxURLs > 0 && len(urls) >= plan.MaxURLs {
				cancel() // 上限到達で graceful 停止
			}
		},
	}

	crawlerOpts, err := types.NewCrawlerOptions(opts)
	if err != nil {
		return engine.CrawlResult{}, fmt.Errorf("katana options: %w", err)
	}
	defer crawlerOpts.Close()

	cr, err := standard.New(crawlerOpts)
	if err != nil {
		return engine.CrawlResult{}, fmt.Errorf("katana engine: %w", err)
	}

	if err := cr.Crawl(scope.BaseURL()); err != nil {
		// MaxURLs 到達による自発 cancel は正常終了扱い。
		if plan.MaxURLs > 0 && len(urls) >= plan.MaxURLs {
			return engine.CrawlResult{URLs: urls, FormCount: forms}, nil
		}
		return engine.CrawlResult{}, fmt.Errorf("katana crawl: %w", err)
	}
	return engine.CrawlResult{URLs: urls, FormCount: forms}, nil
}

// 静的アサーション: Crawler が engine.Crawler を満たすことを保証する。
var _ engine.Crawler = (*Crawler)(nil)
```

> **実 SDK 確認点**（Task 6 Step 2 の `go doc` 出力に合わせる）: `output.Result` のフィールド名
> （`Request` / `Request.Method` / `Request.URL`）、`standard.New` の戻り値に `Close()` があるか。
> `Close()` があれば `cr` にも `defer cr.Close()` を足す。差異があればここを実名に直す。

- [ ] **Step 2: ビルド確認**

Run: `cd worker && go build ./...`
Expected: 成功。

- [ ] **Step 3: integration テストを書く**

`worker/internal/engine/discovery/katana/katana_integration_test.go` を新規作成:

```go
//go:build integration

package katana

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ymd38/goodast/worker/internal/engine"
)

func targetOrDefault() string {
	if v := os.Getenv("NUCLEI_TEST_TARGET"); v != "" {
		return v
	}
	return "http://localhost:3001" // make juiceshop-up の loopback
}

func TestKatanaCrawlDiscoversWithinScope(t *testing.T) {
	target := targetOrDefault()
	scope, err := engine.NewScope(target)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := New().Crawl(ctx, scope, engine.CrawlPlan{Enabled: true, MaxDepth: 2, MaxURLs: 50}, nil)
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}

	// 1. 診断対象が拡張される（seed 1 本より多く発見）＝ hard assert。
	if len(res.URLs) < 1 {
		t.Fatalf("発見 URL が空: %+v", res)
	}
	t.Logf("discovered=%d forms=%d (件数はステートフルで非決定的・レポート)", len(res.URLs), res.FormCount)

	base := scope.Host()
	for _, u := range res.URLs {
		// 2. すべてスコープ内（host:port 一致・危険パス除外）。
		if !scope.Allows(u) {
			t.Errorf("スコープ外 URL が混入: %s", u)
		}
		// 3. 危険パスを踏んでいない。
		low := strings.ToLower(u)
		for _, danger := range []string{"/logout", "/delete", "/admin", "/remove", "/destroy", "/signout"} {
			if strings.Contains(low, danger) {
				t.Errorf("危険パスが発見 URL に含まれる: %s", u)
			}
		}
		// 3'. 別ホストへ出ていない。
		parsed, perr := url.Parse(u)
		if perr != nil || !strings.EqualFold(parsed.Hostname(), base) {
			t.Errorf("別ホストの URL が混入: %s", u)
		}
	}
}

func TestKatanaCrawlRespectsMaxURLs(t *testing.T) {
	target := targetOrDefault()
	scope, err := engine.NewScope(target)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := New().Crawl(ctx, scope, engine.CrawlPlan{Enabled: true, MaxDepth: 3, MaxURLs: 5}, nil)
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	// 4. MaxURLs=5 で頭打ち。
	if len(res.URLs) > 5 {
		t.Fatalf("MaxURLs を超過: %d", len(res.URLs))
	}
}
```

- [ ] **Step 4: integration テストを実行（Juice Shop 起動が前提）**

Run:
```bash
make juiceshop-up
cd worker && go test -tags=integration -race -timeout 8m -run 'TestKatanaCrawl' ./internal/engine/discovery/katana/
```
Expected: PASS（`discovered=N forms=M` がログされ、スコープ外・危険パス・別ホストの混入なし、MaxURLs 遵守）。

> Juice Shop が無い環境では本 Step はスキップし、コミット時に「integration 未実走」を明記する。
> ユニット coverage には影響しない（本ファイルは `//go:build integration`）。

- [ ] **Step 5: コミット**

```bash
git add worker/internal/engine/discovery/katana/
git commit -m "feat(engine): Katana SDK による探索クローラ（engine.Crawler）を実装する"
```

---

### Task 8: scanjob 二段配線（crawl → 動的timeout → scan）と DI

**Files:**
- Modify: `worker/internal/scanjob/worker.go`（`Worker`/`WorkerDeps`/`NewWorker`/`Work`/`runScan`/`executeScan`）
- Modify: `worker/cmd/worker/main.go`（`engine.Crawler` の provider 追加）
- Test: `worker/internal/scanjob/worker_integration_test.go`（追記・fake crawler で二段配線を検証）

**Interfaces:**
- Consumes: `engine.Crawler`、`engine.PlanFor(preset)`（`.Crawl`/`.Timeout`/`.Scan`）、`engine.ScanTimeout`、`jobs.DiscoveryInfo`、`katana.New()`。
- Produces: 二段化された scan ジョブ処理（発見 URL 群を診断・summary に Discovery を格納）。

- [ ] **Step 1: Worker に Crawler 依存を追加**

`worker/internal/scanjob/worker.go`:

```go
type Worker struct {
	river.WorkerDefaults[jobs.ScanArgs]
	queries *db.Queries
	engine  engine.Engine
	crawler engine.Crawler
	cipher  *secrets.Cipher
	logger  *slog.Logger
}

type WorkerDeps struct {
	dig.In
	Queries *db.Queries
	Engine  engine.Engine
	Crawler engine.Crawler
	Cipher  *secrets.Cipher
	Logger  *slog.Logger
}

func NewWorker(d WorkerDeps) *Worker {
	return &Worker{queries: d.Queries, engine: d.Engine, crawler: d.Crawler, cipher: d.Cipher, logger: d.Logger}
}
```

- [ ] **Step 2: `Work` と `runScan` を二段化**

`Work` の profile 導出部（96–101行目付近）を Plan 全体へ変更:

```go
	plan := engine.PlanFor(preset)

	lastAttempt := job.Attempt >= job.MaxAttempts
	return w.runScan(ctx, scanID, pgID, plan, lastAttempt)
```

`runScan` のシグネチャと本体を変更（`profile engine.ScanProfile` → `plan engine.Plan`）。クロール段・動的タイムアウト・Targets 供給・Discovery 集計を挿入:

```go
func (w *Worker) runScan(ctx context.Context, scanID uuid.UUID, pgID pgtype.UUID, plan engine.Plan, lastAttempt bool) error {
	target, err := w.queries.GetScanTarget(ctx, pgID)
	if err != nil {
		return fmt.Errorf("get scan target %s: %w", scanID, err)
	}

	scope, err := engine.NewScope(target.BaseUrl)
	if err != nil {
		w.logger.Error("invalid scan target; marking failed", "scan_id", scanID, "err", err)
		return w.markFailed(ctx, scanID, pgID)
	}

	if scope.RequiresOwnershipVerification() && !target.OwnershipVerified {
		w.logger.Warn("scan target ownership not verified; marking failed",
			"scan_id", scanID, "host", scope.Host())
		return w.markFailed(ctx, scanID, pgID)
	}

	profile := plan.Scan
	if !scope.RequiresOwnershipVerification() {
		profile = profile.ForLocalTarget()
	}

	if err := w.queries.DeleteFindingsByScan(ctx, pgID); err != nil {
		return fmt.Errorf("clear prior findings %s: %w", scanID, err)
	}

	headers, err := w.loadHeaders(ctx, pgID, uuid.UUID(target.SiteID.Bytes))
	if err != nil {
		if permanentCredentialError(err) {
			w.logger.Error("credential decrypt/validation failed; marking failed", "scan_id", scanID, "err", err)
			return w.markFailed(ctx, scanID, pgID)
		}
		if lastAttempt {
			w.logger.Error("credential load failed on final attempt; marking failed", "scan_id", scanID, "err", err)
			return w.markFailed(ctx, scanID, pgID)
		}
		return fmt.Errorf("load credentials %s: %w", scanID, err)
	}
	if len(headers) > 0 {
		w.logger.Info("authenticated scan; injecting session headers", "scan_id", scanID, "header_count", len(headers))
	}

	// クロール段: 発見 URL 群を診断対象にする。失敗・0件は単一 URL フォールバック（診断は続行）。
	targets := []string{scope.BaseURL()}
	var discovery *jobs.DiscoveryInfo
	if plan.Crawl.Enabled {
		res, cerr := w.crawler.Crawl(ctx, scope, plan.Crawl, headers)
		if cerr != nil {
			w.logger.Warn("crawl failed; falling back to single URL", "scan_id", scanID, "err", cerr)
		} else if len(res.URLs) > 0 {
			targets = res.URLs
			discovery = &jobs.DiscoveryInfo{URLCount: len(res.URLs), FormCount: res.FormCount}
			w.logger.Info("crawl complete", "scan_id", scanID, "urls", len(res.URLs), "forms", res.FormCount)
		}
	}

	// 動的タイムアウト: 発見 URL 数に応じ、preset CEILING（plan.Timeout）を上限に scan 段の枠を決める。
	scanCtx, cancel := context.WithTimeout(ctx, engine.ScanTimeout(len(targets), plan.Timeout))
	defer cancel()

	findings, err := w.executeScan(scanCtx, pgID, scope, headers, profile, targets)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(scanCtx.Err(), context.DeadlineExceeded) {
			w.logger.Error("scan timed out; marking failed", "scan_id", scanID, "err", err)
			return w.markFailed(ctx, scanID, pgID)
		}
		if lastAttempt {
			w.logger.Error("scan failed on final attempt; marking failed", "scan_id", scanID, "err", err)
			return w.markFailed(ctx, scanID, pgID)
		}
		return fmt.Errorf("scan %s: %w", scanID, err)
	}

	payload, err := json.Marshal(jobs.ScanSummary{Findings: engine.Summarize(findings), Discovery: discovery})
	if err != nil {
		return fmt.Errorf("marshal summary %s: %w", scanID, err)
	}

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
```

`executeScan` に `targets []string` を追加し `ScanRequest.Targets` に載せる:

```go
func (w *Worker) executeScan(ctx context.Context, pgID pgtype.UUID, scope engine.Scope, headers []string, profile engine.ScanProfile, targets []string) ([]engine.Finding, error) {
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

	if err := w.engine.Scan(ctx, engine.ScanRequest{Scope: scope, Targets: targets, Headers: headers, Profile: profile}, onFinding); err != nil {
		return nil, err
	}
	if saveErr != nil {
		return nil, saveErr
	}
	return collected, nil
}
```

- [ ] **Step 3: DI provider を追加**

`worker/cmd/worker/main.go` の `providers` に追加（`katana` を import）:

```go
import (
	// 既存に加える
	"github.com/ymd38/goodast/worker/internal/engine/discovery/katana"
)
```

```go
		func() engine.Engine { return nuclei.New() },
		// engine.Crawler の実装は Katana（standard engine・探索専用）。per-scan で状態を持たない。
		func() engine.Crawler { return katana.New() },
```

- [ ] **Step 4: ビルド確認**

Run: `cd worker && go build ./...`
Expected: 成功。

- [ ] **Step 5: 二段配線の integration テストを追加**

まず `worker/internal/scanjob/worker_integration_test.go` を読み、既存の DB/Worker セットアップヘルパ（`NewWorker(WorkerDeps{...})` の組み立てとテスト用 scan/site 投入）を把握する。既存パターンに沿って fake crawler を注入するテストを追記する:

```go
// fakeCrawler は二段配線検証用の決定的クローラ（実クロール不要）。
type fakeCrawler struct {
	res engine.CrawlResult
	err error
}

func (f fakeCrawler) Crawl(_ context.Context, _ engine.Scope, _ engine.CrawlPlan, _ []string) (engine.CrawlResult, error) {
	return f.res, f.err
}
func (f fakeCrawler) Version() string { return "fake/v0" }
```

テスト本体（既存ヘルパの名前は worker_integration_test.go の実装に合わせる。ローカル対象＝所有確認スキップで standard preset を使い、fake が返す URL 群で診断されることと summary.Discovery を検証する）:

```go
func TestWorkerTwoPhaseDiscovery(t *testing.T) {
	// --- 既存ヘルパで throwaway PG・site(localhost)・queued scan(preset=standard) を用意する ---
	//     （worker_integration_test.go の既存セットアップと同じ組み立て方に従う）
	// 例: q := newTestQueries(t); siteID := insertLocalVerifiedSite(t, q); scanID := insertQueuedScan(t, q, siteID, "standard")

	fake := fakeCrawler{res: engine.CrawlResult{
		URLs:      []string{"http://localhost:3001/", "http://localhost:3001/products"},
		FormCount: 2,
	}}
	w := NewWorker(WorkerDeps{
		Queries: q,
		Engine:  stubEngine{}, // 既存テストで使う finding を返さない/固定するスタブ engine に合わせる
		Crawler: fake,
		Cipher:  testCipher(t),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	job := &river.Job[jobs.ScanArgs]{ /* Args: {ScanID: scanID, Preset: "standard"}, Attempt:1, MaxAttempts:3 */ }
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	// scan が done になり summary.Discovery に fake の集計が載る。
	scan := getScan(t, q, scanID) // 既存ヘルパ
	var sum jobs.ScanSummary
	if err := json.Unmarshal(scan.SummaryJson, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Discovery == nil || sum.Discovery.URLCount != 2 || sum.Discovery.FormCount != 2 {
		t.Fatalf("Discovery = %+v; want url=2 form=2", sum.Discovery)
	}
}
```

> このテストは既存の scanjob integration ハーネス（throwaway PG・stub engine）を再利用する。
> `stubEngine` / `testCipher` / scan 投入ヘルパは既存テストのものを使う（無ければ既存 `TestWorker…`
> の組み立てを参考に最小限で用意）。**実クロールはしない**（fake crawler なので Juice Shop 不要）。

- [ ] **Step 6: テストを実行**

Run:
```bash
cd worker && go test -tags=integration -race -timeout 5m -run 'TestWorkerTwoPhaseDiscovery' ./internal/scanjob/
```
Expected: PASS（scan=done・`Discovery{URLCount:2, FormCount:2}`）。既存の scanjob integration テストも継続 PASS:
`cd worker && go test -tags=integration -race ./internal/scanjob/`

- [ ] **Step 7: コミット**

```bash
git add worker/internal/scanjob/worker.go worker/cmd/worker/main.go worker/internal/scanjob/worker_integration_test.go
git commit -m "feat(worker): スキャンジョブを crawl→動的timeout→scan の二段に配線する"
```

---

### Task 9: Makefile ターゲット・カバレッジ除外・ドキュメント更新

**Files:**
- Modify: `Makefile`（`cover` の除外に katana 追加・`discovery-scan` ターゲット追加）
- Modify: `PROGRESS.md`
- Modify: `docs/poc-plan.md`（該当フェーズの記述があれば）

**Interfaces:**
- Consumes: Task 7 の integration テスト。

- [ ] **Step 1: カバレッジ除外に katana を追加**

`Makefile` の `cover` ターゲット（123行目付近）の grep 除外に `/engine/discovery/katana$$` を追加:

```makefile
		$$(go list ./... | grep -v '/db$$\|/cmd/\|/engine/nuclei$$\|/engine/discovery/katana$$\|/scanjob$$') \
```

- [ ] **Step 2: `discovery-scan` ターゲットを追加**

`Makefile` の nuclei integration ターゲット群（154行目付近）の並びに追加（`juiceshop-up` 前提・既存 `nuclei-scan` に倣う）:

```makefile
.PHONY: discovery-scan
discovery-scan: ## Katana 探索の integration テスト（要 juiceshop-up。NUCLEI_TEST_TARGET 上書き可）
	cd worker && go test -tags=integration -v -timeout 8m -run TestKatanaCrawl ./internal/engine/discovery/katana/
```

- [ ] **Step 3: ユニットカバレッジが 100% を維持することを確認**

Run: `make cover`
Expected: `total` の statements が 100%（新規純粋コード `crawl.go` / `preset.go` 追加分・`DangerousPathRegexes`/`ScanTimeout`/`TargetsOrBase`/`PlanFor` crawl 分岐が網羅されている）。katana アダプタは除外済み。

> 100% に届かない場合は Task 1–4 の純粋関数に未網羅分岐が無いか確認し、テーブル駆動テストに
> ケースを足してから次へ進む（新規の純粋コードはすべてユニットで網羅する方針）。

- [ ] **Step 4: PROGRESS.md を更新**

`PROGRESS.md` の「現在地スナップショット」と「直近のアクション」に、Katana 探索コア実装（発見 URL 群を Nuclei 診断へ拡張・二段配線・summary に Discovery counts・フォームファジングと `discovered_endpoints` テーブルは次タスク）を追記する。spec への参照リンク（`docs/superpowers/specs/2026-07-23-katana-discovery-design.md`）を含める。

- [ ] **Step 5: docs/poc-plan.md を確認・更新**

`docs/poc-plan.md` に Phase 2 探索の記述があれば、Katana SDK 採用・探索コア完了・フォームファジングは次段、という現状に更新する（該当箇所が無ければ本 Step はスキップし、その旨コミットメッセージに記す）。

- [ ] **Step 6: 全体テスト・lint の最終確認**

Run:
```bash
cd worker && go test ./... -race
cd worker && GOTOOLCHAIN=go1.26.4 golangci-lint run
cd jobs && go test ./... -race
```
Expected: すべて PASS・lint 0 issues。

- [ ] **Step 7: コミット**

```bash
git add Makefile PROGRESS.md docs/poc-plan.md
git commit -m "docs(worker): Katana 探索の make ターゲット・カバレッジ除外・進捗を更新する"
```

---

## Self-Review（spec 突合）

**1. Spec coverage:**
- §2 Katana 独立 Crawler 隔離 → Task 4（interface）+ Task 7（実装）✓
- §3 クロール段ガード（native scope＋後段チェック＋GET-only＋MaxURLs cancel）→ Task 7 ✓／危険パス regex → Task 4 ✓
- §3.2 Nuclei 段 post-filter 維持・変更は Targets 加算のみ → Task 3 ✓
- §4 パッケージ配置（crawl.go / discovery/katana）→ Task 1–4, 7 ✓
- §4.1 型（CrawlPlan / CrawlResult / Crawler）→ Task 1, 4 ✓
- §5 二段オーケストレーション → Task 8 ✓／§5.1 TargetsOrBase → Task 3 ✓／§5.2 ScanSummary.Discovery → Task 5 ✓／§5.3 DI → Task 8 ✓
- §6 preset クロール上限 → Task 1 ✓／§6.1 ScanTimeout → Task 2 ✓
- §7 エラー（クロール失敗/0件フォールバック・認証クロール headers）→ Task 8（runScan）+ Task 7（headers）✓
- §8 テスト（純粋 unit / Juice Shop integration）→ Task 1–5（unit）, 7, 8（integration）✓
- §9 依存・固定 → Task 6 ✓
- §10 前方互換（単一 URL 挙動不変・omitempty）→ Task 3, 5 ✓

**2. Placeholder scan:** `version = "katana/vX.Y.Z"` と Task 6 の解決版は実 `go get` 結果に置換する明示指示あり。Task 8 Step 5 の既存ヘルパ名は worker_integration_test.go を読んで合わせる指示あり（logic のプレースホルダではなく既存ハーネス再利用）。SDK フィールド名は Task 6 Step 2 の `go doc` を正とする明示あり。

**3. Type consistency:** `CrawlPlan`/`CrawlResult`/`DiscoveryInfo`/`ScanTimeout`/`TargetsOrBase`/`DangerousPathRegexes` の名称・シグネチャは全タスクで一致。`runScan(plan engine.Plan)` / `executeScan(..., targets []string)` の変更はシグネチャ変更に追随（Task 8 内で一貫）。
