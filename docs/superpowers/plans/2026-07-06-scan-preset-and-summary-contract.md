# スキャンプリセット正式対応 + summary_json ドリフト対策 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** スキャンプリセット（軽量/標準/詳細）を選択可能にし、タグ・ジョブタイムアウトのハードコードを撤廃、あわせて `summary_json` の worker/api 間ドリフトを共有型で構造的に固定する。

**Architecture:** プリセット識別子 enum は SDK 非依存の共有モジュール `jobs/` に置き（ADR-0002 により api は engine を import 不可のため）、preset→実行パラメータ（タグ/レート/タイムアウト）の写像は `engine` に置く。preset は enqueue 時に `jobs.ScanArgs` に載せて river の Timeout callback（context/DB 不可）に届けつつ、`scans.preset` カラムにも永続化する。`summary_json` は `jobs.ScanSummary` を worker/api 双方が経由することで形を一元化する。

**Tech Stack:** Go 1.26.4 / Gin / sqlc + pgx v5 / golang-migrate / river / testify / go.uber.org/dig / Nuclei v3 SDK（worker のみ）

## Global Constraints

- Nuclei SDK の import は `worker/internal/engine/nuclei` のみ（ADR-0002）。api は `worker/internal/engine` を import しない。
- 生 SQL を Go に直書きしない。SQL は `*/db/queries/*.sql`、`sqlc generate` で生成（sqlc v1.31.1）。生成コード（`internal/db/*.go`）は編集しない。
- マイグレーションは `migrations/` に連番で追加。AutoMigrate 禁止。変更後は両モジュールで `sqlc generate`。
- エラーは `fmt.Errorf("...: %w", err)` でラップ。`context.Context` を全レイヤーで伝播。
- ログは `log/slog`。認証情報・全フィールドの平文ログ禁止。
- 純粋ロジックは unit カバレッジ C0 100%（`jobs` / `engine` パッケージ）。Nuclei SDK を呼ぶテストは `//go:build integration`。
- テーブル駆動テスト + `-race`。
- プリセット値: rate 全 preset 10req/s、`exclude dos,intrusive` 共通。light=`misconfig,tech,exposure`/5分、standard=light+`exposed-panels,default-login,cve`/15分、deep=standard+`xss,sqli,lfi,ssrf,rce,takeover`/30分。DefaultPreset=standard。
- モジュール path: `github.com/ymd38/goodast/{jobs,api,worker,secrets}`。`engine` は `worker/internal/engine`。
- 各タスクの検証は該当モジュールで実行: api は `cd api && go test ./... -race`、worker は `cd worker && go test ./... -race`、jobs は `cd jobs && go test ./... -race`。lint は `golangci-lint run`。

---

### Task 1: `jobs.Preset` 型と検証

プリセット識別子の enum を共有モジュールに新設する。api・worker の双方が参照する唯一の定義。

**Files:**
- Create: `jobs/preset.go`
- Test: `jobs/preset_test.go`

**Interfaces:**
- Produces:
  - `type Preset string`
  - `const PresetLight Preset = "light"`, `PresetStandard = "standard"`, `PresetDeep = "deep"`
  - `const DefaultPreset = PresetStandard`
  - `func ParsePreset(s string) (Preset, error)` — 空文字は `DefaultPreset` を返す（省略許容）。未知値は `ErrInvalidPreset` を返す。
  - `var ErrInvalidPreset = errors.New("jobs: invalid scan preset")`
  - `func (p Preset) String() string`

- [ ] **Step 1: Write the failing test**

`jobs/preset_test.go`:
```go
package jobs

import (
	"errors"
	"testing"
)

func TestParsePreset(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Preset
		wantErr error
	}{
		{"light", "light", PresetLight, nil},
		{"standard", "standard", PresetStandard, nil},
		{"deep", "deep", PresetDeep, nil},
		{"empty defaults to standard", "", PresetStandard, nil},
		{"unknown", "aggressive", "", ErrInvalidPreset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePreset(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPresetString(t *testing.T) {
	if PresetDeep.String() != "deep" {
		t.Fatalf("String() = %q", PresetDeep.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd jobs && go test ./... -run TestParsePreset -v`
Expected: FAIL（`undefined: ParsePreset` 等でコンパイル不可）

- [ ] **Step 3: Write minimal implementation**

`jobs/preset.go`:
```go
package jobs

import "errors"

// Preset はスキャンの範囲・深さのプリセット（企画書 §6-2 のウィザード選択）。
// api（検証・保存）と worker（Timeout・scan config）が共有するため、SDK 非依存の
// 本モジュールに一元定義する（ADR-0002: api は engine を import できない）。
type Preset string

const (
	PresetLight    Preset = "light"    // 軽量: 素早い基本チェック
	PresetStandard Preset = "standard" // 標準: 実用的な中間（デフォルト）
	PresetDeep     Preset = "deep"     // 詳細: 広いタグ集合（タグ有界で全テンプレは回さない）
)

// DefaultPreset は preset 省略時に採用する安全な既定値。
const DefaultPreset = PresetStandard

// ErrInvalidPreset は未知の preset 文字列を表す。
var ErrInvalidPreset = errors.New("jobs: invalid scan preset")

// ParsePreset は文字列を Preset に変換する。空文字は DefaultPreset を返し（省略許容）、
// 未知値は ErrInvalidPreset を返す。DB CHECK 制約・HTTP 400 と二重に不正値を弾く。
func ParsePreset(s string) (Preset, error) {
	switch Preset(s) {
	case PresetLight, PresetStandard, PresetDeep:
		return Preset(s), nil
	case "":
		return DefaultPreset, nil
	default:
		return "", ErrInvalidPreset
	}
}

// String は Preset の文字列表現を返す。
func (p Preset) String() string { return string(p) }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd jobs && go test ./... -race -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add jobs/preset.go jobs/preset_test.go
git commit -m "feat(jobs): スキャンプリセット識別子の共有型 Preset を追加する"
```

---

### Task 2: `jobs.ScanSummary` 共有 wire 型（ドリフト対策の土台）

`summary_json` の canonical な形を共有モジュールに新設する。worker が書き、api が読む唯一の契約。

**Files:**
- Create: `jobs/summary.go`
- Test: `jobs/summary_test.go`

**Interfaces:**
- Produces:
  - `type SeverityCounts struct { Critical, High, Medium, Low, Info, Total int }`（json タグ: `critical`/`high`/`medium`/`low`/`info`/`total`）
  - `type ScanSummary struct { Findings SeverityCounts }`（json タグ: `findings`）

- [ ] **Step 1: Write the failing test**

`jobs/summary_test.go`:
```go
package jobs

import (
	"encoding/json"
	"testing"
)

func TestScanSummaryRoundTrip(t *testing.T) {
	in := ScanSummary{Findings: SeverityCounts{
		Critical: 1, High: 2, Medium: 3, Low: 4, Info: 5, Total: 15,
	}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ScanSummary
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, in)
	}
}

func TestScanSummaryWireShapeIsNested(t *testing.T) {
	raw, _ := json.Marshal(ScanSummary{Findings: SeverityCounts{Critical: 1}})
	// worker が書き api が読む形を固定: {"findings":{"critical":1,...}}
	var probe struct {
		Findings struct {
			Critical int `json:"critical"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("unmarshal probe: %v", err)
	}
	if probe.Findings.Critical != 1 {
		t.Fatalf("expected nested findings.critical=1, got %d", probe.Findings.Critical)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd jobs && go test ./... -run TestScanSummary -v`
Expected: FAIL（`undefined: ScanSummary`）

- [ ] **Step 3: Write minimal implementation**

`jobs/summary.go`:
```go
package jobs

// SeverityCounts は 1 スキャンの重大度別 finding 件数。scans.summary_json の中身。
// worker（engine 集計）と api（スコア計算）の双方がこの型を経由することで、
// summary_json の形が別々に定義されてズレる（ドリフト）のを構造的に防ぐ。
type SeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

// ScanSummary は scans.summary_json に保存するエンベロープ。件数を "findings" キーの
// 下にネストする。スコアは持たない（api 側 report が SeverityCounts から算出する）。
type ScanSummary struct {
	Findings SeverityCounts `json:"findings"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd jobs && go test ./... -race -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add jobs/summary.go jobs/summary_test.go
git commit -m "feat(jobs): summary_json の共有 wire 型 ScanSummary を追加する"
```

---

### Task 3: `jobs.ScanArgs` に Preset を載せる

river ジョブ引数に preset を追加。Timeout callback（DB 不可）が preset を得る経路。

**Files:**
- Modify: `jobs/scanargs.go`
- Test: `jobs/scanargs_test.go`（新規）

**Interfaces:**
- Consumes: `jobs.Preset`（Task 1）
- Produces: `ScanArgs{ ScanID string; Preset Preset }`（Preset の json タグ `preset`）

- [ ] **Step 1: Write the failing test**

`jobs/scanargs_test.go`:
```go
package jobs

import (
	"encoding/json"
	"testing"
)

func TestScanArgsCarriesPreset(t *testing.T) {
	raw, err := json.Marshal(ScanArgs{ScanID: "abc", Preset: PresetDeep})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ScanArgs
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ScanID != "abc" || out.Preset != PresetDeep {
		t.Fatalf("got %+v", out)
	}
}

func TestScanArgsKind(t *testing.T) {
	if (ScanArgs{}).Kind() != "scan" {
		t.Fatalf("kind changed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd jobs && go test ./... -run TestScanArgs -v`
Expected: FAIL（`unknown field Preset`）

- [ ] **Step 3: Write minimal implementation**

`jobs/scanargs.go` の `ScanArgs` を次に置き換える:
```go
// ScanArgs はスキャン実行ジョブの引数。worker は ScanID から scan / site / credentials を
// DB ロードする。Preset は river の Timeout callback（context/DB を持てない）が
// タイムアウト決定に使うため、DB カラムとは別にジョブ引数にも載せる。
type ScanArgs struct {
	ScanID string `json:"scan_id"`
	Preset Preset `json:"preset"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd jobs && go test ./... -race -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add jobs/scanargs.go jobs/scanargs_test.go
git commit -m "feat(jobs): ScanArgs に Preset を追加し Timeout callback へ届ける"
```

---

### Task 4: `engine.PlanFor` — preset → 実行パラメータの写像

preset をタグ/レート/タイムアウトに落とす純粋ロジックを engine に追加。unit 100%。

**Files:**
- Create: `worker/internal/engine/preset.go`
- Test: `worker/internal/engine/preset_test.go`

**Interfaces:**
- Consumes: `jobs.Preset`（Task 1）
- Produces:
  - `type ScanProfile struct { Tags, ExcludeTags []string; Severities string; RateLimit int; RatePeriod time.Duration }`
  - `type Plan struct { Scan ScanProfile; Timeout time.Duration }`
  - `func PlanFor(p jobs.Preset) Plan` — 未知/空 preset は standard の Plan を返す（防御的）。

- [ ] **Step 1: Write the failing test**

`worker/internal/engine/preset_test.go`:
```go
package engine

import (
	"testing"
	"time"

	"github.com/ymd38/goodast/jobs"
)

func TestPlanFor(t *testing.T) {
	tests := []struct {
		name        string
		preset      jobs.Preset
		wantTags    []string
		wantTimeout time.Duration
	}{
		{
			"light",
			jobs.PresetLight,
			[]string{"misconfig", "tech", "exposure"},
			5 * time.Minute,
		},
		{
			"standard",
			jobs.PresetStandard,
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve"},
			15 * time.Minute,
		},
		{
			"deep",
			jobs.PresetDeep,
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve", "xss", "sqli", "lfi", "ssrf", "rce", "takeover"},
			30 * time.Minute,
		},
		{
			"unknown falls back to standard",
			jobs.Preset("bogus"),
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve"},
			15 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := PlanFor(tt.preset)
			if tt.wantTimeout != plan.Timeout {
				t.Fatalf("timeout = %v, want %v", plan.Timeout, tt.wantTimeout)
			}
			if !equalStrings(plan.Scan.Tags, tt.wantTags) {
				t.Fatalf("tags = %v, want %v", plan.Scan.Tags, tt.wantTags)
			}
			if !equalStrings(plan.Scan.ExcludeTags, []string{"dos", "intrusive"}) {
				t.Fatalf("exclude = %v", plan.Scan.ExcludeTags)
			}
			if plan.Scan.RateLimit != 10 || plan.Scan.RatePeriod != time.Second {
				t.Fatalf("rate = %d/%v", plan.Scan.RateLimit, plan.Scan.RatePeriod)
			}
			if plan.Scan.Severities != "" {
				t.Fatalf("severities = %q", plan.Scan.Severities)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd worker && go test ./internal/engine/ -run TestPlanFor -v`
Expected: FAIL（`undefined: PlanFor`）

- [ ] **Step 3: Write minimal implementation**

`worker/internal/engine/preset.go`:
```go
package engine

import (
	"time"

	"github.com/ymd38/goodast/jobs"
)

// ScanProfile は 1 スキャンの実行パラメータ（テンプレート選択とレート）。
// preset から導出され、ScanRequest 経由でエンジンに渡る。per-scan で可変。
type ScanProfile struct {
	Tags        []string
	ExcludeTags []string
	Severities  string
	RateLimit   int
	RatePeriod  time.Duration
}

// Plan は preset から導出したスキャン実行計画。Scan はエンジンへ、Timeout は
// river ジョブの実行上限（scanjob.Worker.Timeout）へ渡る。
type Plan struct {
	Scan    ScanProfile
	Timeout time.Duration
}

// 破壊的テンプレートは全 preset で除外（ADR / Critical Constraints）。
var excludeDestructive = []string{"dos", "intrusive"}

// PlanFor は preset を実行計画に写像する。未知・空 preset は安全側として standard を返す
// （api で検証済みのため通常は起きないが、防御的に既定へ倒す）。
func PlanFor(p jobs.Preset) Plan {
	switch p {
	case jobs.PresetLight:
		return plan([]string{"misconfig", "tech", "exposure"}, 5*time.Minute)
	case jobs.PresetDeep:
		return plan([]string{
			"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve",
			"xss", "sqli", "lfi", "ssrf", "rce", "takeover",
		}, 30*time.Minute)
	default: // standard を既定に
		return plan([]string{
			"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve",
		}, 15*time.Minute)
	}
}

// plan は共通のレート・除外タグを載せた Plan を組み立てる（DRY）。
func plan(tags []string, timeout time.Duration) Plan {
	return Plan{
		Scan: ScanProfile{
			Tags:        tags,
			ExcludeTags: excludeDestructive,
			Severities:  "",
			RateLimit:   10,
			RatePeriod:  time.Second,
		},
		Timeout: timeout,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd worker && go test ./internal/engine/ -race -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add worker/internal/engine/preset.go worker/internal/engine/preset_test.go
git commit -m "feat(engine): preset を実行計画に写像する PlanFor を追加する"
```

---

### Task 5: `engine.Summarize` を `jobs.SeverityCounts` 返却に切り替える

engine の集計出力を共有 wire 型に統一し、`engine.Summary` を廃止する。

**Files:**
- Modify: `worker/internal/engine/summary.go`
- Modify: `worker/internal/engine/summary_test.go`

**Interfaces:**
- Consumes: `jobs.SeverityCounts`（Task 2）
- Produces: `func Summarize(findings []Finding) jobs.SeverityCounts`（`engine.Summary` は削除）

- [ ] **Step 1: Update the test to expect jobs.SeverityCounts**

`worker/internal/engine/summary_test.go` を開き、`engine.Summary` / `Summary{...}` への参照を `jobs.SeverityCounts` に置換する。先頭に import 追加:
```go
import (
	"testing"

	"github.com/ymd38/goodast/jobs"
)
```
アサーション中の `Summary{Critical: 1, ...}` を `jobs.SeverityCounts{Critical: 1, ...}` に、`Summarize(...)` の戻り値型比較を `jobs.SeverityCounts` に合わせる。既存のケース値（件数）はそのまま維持する。

- [ ] **Step 2: Run test to verify it fails**

Run: `cd worker && go test ./internal/engine/ -run TestSummarize -v`
Expected: FAIL（`Summary` 未定義、または型不一致）

- [ ] **Step 3: Rewrite summary.go**

`worker/internal/engine/summary.go` を全置換:
```go
package engine

import "github.com/ymd38/goodast/jobs"

// Summarize は findings を重大度別に集計する純粋関数。集計結果は scans.summary_json の
// 共有 wire 型 jobs.SeverityCounts で返し、api（スコア計算）と形を共有する（ドリフト防止）。
func Summarize(findings []Finding) jobs.SeverityCounts {
	var s jobs.SeverityCounts
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			s.Critical++
		case SeverityHigh:
			s.High++
		case SeverityMedium:
			s.Medium++
		case SeverityLow:
			s.Low++
		case SeverityInfo:
			s.Info++
		}
	}
	s.Total = len(findings)
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd worker && go test ./internal/engine/ -race -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add worker/internal/engine/summary.go worker/internal/engine/summary_test.go
git commit -m "refactor(engine): Summarize を共有型 jobs.SeverityCounts 返却に統一する"
```

---

### Task 6: `engine.ScanRequest` に Profile を追加

per-scan の実行パラメータを ScanRequest で運ぶ。エンジン実装が参照する境界を用意する。

**Files:**
- Modify: `worker/internal/engine/engine.go`

**Interfaces:**
- Consumes: `engine.ScanProfile`（Task 4）
- Produces: `ScanRequest{ Scope Scope; Headers []string; Profile ScanProfile }`

- [ ] **Step 1: Modify ScanRequest**

`worker/internal/engine/engine.go` の `ScanRequest` に `Profile` フィールドを追加:
```go
// ScanRequest は 1 回のスキャン要求。対象・許可境界・実行パラメータを内包する。
type ScanRequest struct {
	// Scope はスキャン対象の許可境界（allowlist）。エンジンはこの外へ逸脱しない。
	Scope Scope
	// Headers は全リクエストに付与する認証ヘッダ（"Name: Value" 形式）。未認証時は空。
	Headers []string
	// Profile は preset 由来の実行パラメータ（テンプレート選択・レート）。
	Profile ScanProfile
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd worker && go build ./internal/engine/`
Expected: 成功（nuclei パッケージはまだ旧 cfg 参照でこの時点ではビルドしない。engine 単体のみ確認）

- [ ] **Step 3: Commit**

```bash
git add worker/internal/engine/engine.go
git commit -m "feat(engine): ScanRequest に preset 由来の Profile を追加する"
```

---

### Task 7: `nuclei.Scan` を `req.Profile` 参照へ、DefaultConfig を撤廃

Nuclei アダプタからハードコード config を除去し、per-scan の Profile を使う。

**Files:**
- Modify: `worker/internal/engine/nuclei/nuclei.go`
- Modify: `worker/cmd/worker/main.go:54`

**Interfaces:**
- Consumes: `engine.ScanRequest.Profile`（Task 6）
- Produces: `func New() *Engine`（引数なし・`nuclei.Config`/`DefaultConfig` は削除）

- [ ] **Step 1: Rewrite nuclei.go config handling**

`worker/internal/engine/nuclei/nuclei.go` を次の要領で編集する。

(a) `Config` 型定義（24-37 行あたり）と `DefaultConfig()`（39-54 行）を**削除**する。

(b) `Engine` 型と `New`/`Version` を次に置換:
```go
// Engine は engine.Engine の Nuclei 実装。実行パラメータは per-scan で ScanRequest.Profile
// から受け取るため、Engine 自体は状態を持たない（プリセット差はリクエスト側で表現する）。
type Engine struct{}

// New は Nuclei エンジンを生成する。
func New() *Engine { return &Engine{} }

// Version は固定された Nuclei SDK バージョン識別子を返す。
func (e *Engine) Version() string { return version }
```

(c) `Scan` の冒頭 `opts` 構築で `e.cfg.*` を `req.Profile.*` に置換:
```go
func (e *Engine) Scan(ctx context.Context, req engine.ScanRequest, onFinding engine.FindingCallback) error {
	opts := []nucleilib.NucleiSDKOptions{
		nucleilib.WithTemplateFilters(nucleilib.TemplateFilters{
			Severity:    req.Profile.Severities,
			Tags:        req.Profile.Tags,
			ExcludeTags: req.Profile.ExcludeTags,
		}),
		nucleilib.WithGlobalRateLimit(req.Profile.RateLimit, req.Profile.RatePeriod),
		nucleilib.WithSandboxOptions(false, false),
		nucleilib.DisableUpdateCheck(),
	}
	// 認証後スキャンの DisableRedirects 分岐（既存 115-120 行）は変更しない。
	// ...（以降は既存のまま）
}
```
※ `time` import が `Config` 削除後に未使用にならないか確認する。`Scan` 内で `time` を使っていなければ import から削除する（`goimports`/`golangci-lint run --fix` で自動整理可）。

- [ ] **Step 2: Update worker main wiring**

`worker/cmd/worker/main.go:54` を置換:
```go
		func() engine.Engine { return nuclei.New() },
```

- [ ] **Step 3: Verify build**

Run: `cd worker && go build ./...`
Expected: 成功（scanjob はまだ旧 Summarize 呼び出しだが Task 5 で jobs.SeverityCounts 化済み・payload marshal は Task 8 で対応。この時点で scanjob がビルドエラーなら Task 8 のマーシャル修正を先に取り込む必要がある——順序に注意）

> 注: 本タスクは engine/nuclei と main のみ。scanjob の `scanSummary`（Task 8）とは独立にビルドできるよう、Task 8 を続けて実施する。

- [ ] **Step 4: Commit**

```bash
git add worker/internal/engine/nuclei/nuclei.go worker/cmd/worker/main.go
git commit -m "refactor(engine): nuclei を per-scan Profile 参照にしハードコード config を撤廃する"
```

---

### Task 8: `scanjob.Worker` を Profile 配線 + preset タイムアウト + 共有 summary へ

worker のオーケストレーションを preset 対応にする。Timeout を PlanFor 化、Scan に Profile を渡し、summary を `jobs.ScanSummary` で書く。

**Files:**
- Modify: `worker/internal/scanjob/worker.go`

**Interfaces:**
- Consumes: `engine.PlanFor`（Task 4）, `engine.ScanRequest.Profile`（Task 6）, `jobs.ScanSummary`/`jobs.ParsePreset`（Task 1/2）
- Produces: preset を反映した scan 実行

- [ ] **Step 1: Replace Timeout to use preset**

`worker.go` の `Timeout`（50-55 行）を置換:
```go
// Timeout は scan ジョブ1回あたりの実行上限。preset ごとに engine.PlanFor が返す値を使う。
// river の callback は context/DB を持てないため、preset はジョブ引数から得る。
func (w *Worker) Timeout(job *river.Job[jobs.ScanArgs]) time.Duration {
	preset, _ := jobs.ParsePreset(string(job.Args.Preset))
	return engine.PlanFor(preset).Timeout
}
```

- [ ] **Step 2: Delete scanSummary struct**

`worker.go` の `scanSummary` 型定義（57-61 行）を**削除**する（`jobs.ScanSummary` に置き換わる）。

- [ ] **Step 3: Pass preset through Work → runScan → executeScan**

`Work`（68 行〜）で preset を解決し runScan に渡す。`runScan` と `executeScan` のシグネチャに `profile engine.ScanProfile` を追加する。

`Work` 内、`lastAttempt` 算出の直前に preset 解決を追加:
```go
	preset, err := jobs.ParsePreset(string(job.Args.Preset))
	if err != nil {
		// api で検証済みのため通常起きない。不正なら設定不備として failed に確定する。
		w.logger.Error("invalid preset on scan job; marking failed", "scan_id", scanID, "preset", job.Args.Preset)
		return w.markFailed(ctx, scanID, pgID)
	}
	profile := engine.PlanFor(preset).Scan

	lastAttempt := job.Attempt >= job.MaxAttempts
	return w.runScan(ctx, scanID, pgID, profile, lastAttempt)
```

`runScan` シグネチャを変更:
```go
func (w *Worker) runScan(ctx context.Context, scanID uuid.UUID, pgID pgtype.UUID, profile engine.ScanProfile, lastAttempt bool) error {
```
その中の `executeScan` 呼び出し（152 行付近）を:
```go
	findings, err := w.executeScan(ctx, pgID, scope, headers, profile)
```

`executeScan` シグネチャと engine 呼び出しを変更:
```go
func (w *Worker) executeScan(ctx context.Context, pgID pgtype.UUID, scope engine.Scope, headers []string, profile engine.ScanProfile) ([]engine.Finding, error) {
	// ...（mu / collected / onFinding は既存のまま）
	if err := w.engine.Scan(ctx, engine.ScanRequest{Scope: scope, Headers: headers, Profile: profile}, onFinding); err != nil {
		return nil, err
	}
	// ...
}
```

- [ ] **Step 4: Marshal jobs.ScanSummary**

`runScan` 内の payload marshal（163 行付近）を置換:
```go
	payload, err := json.Marshal(jobs.ScanSummary{Findings: engine.Summarize(findings)})
```

- [ ] **Step 5: Verify build**

Run: `cd worker && go build ./...`
Expected: 成功

- [ ] **Step 6: Run worker unit + lint**

Run: `cd worker && go test ./... -race` then `golangci-lint run`
Expected: PASS（integration テストは `//go:build integration` なので通常 test には含まれない）

- [ ] **Step 7: Commit**

```bash
git add worker/internal/scanjob/worker.go
git commit -m "feat(scanjob): preset を Profile/Timeout に反映し summary を共有型で書く"
```

---

### Task 9: DB マイグレーション + sqlc（scans.preset）

`scans` に preset カラムを追加し、両モジュールの `CreateScan` に preset 引数を通す。

**Files:**
- Create: `migrations/000006_scans_preset.up.sql`
- Create: `migrations/000006_scans_preset.down.sql`
- Modify: `api/internal/db/queries/scans.sql`（CreateScan）
- Regenerate: `api/internal/db/*.go`, `worker/internal/db/*.go`（sqlc）

**Interfaces:**
- Produces: `db.CreateScanParams{ SiteID, Preset }`（api 側 sqlc 生成）

- [ ] **Step 1: Write migration**

`migrations/000006_scans_preset.up.sql`:
```sql
-- scans に実行プリセット（軽量/標準/詳細）を記録する。履歴・ダッシュボード表示の記録用。
-- 既存行は安全側の standard で埋める。CHECK は jobs.Preset の値集合と一致させる。
ALTER TABLE scans
    ADD COLUMN preset text NOT NULL DEFAULT 'standard'
    CHECK (preset IN ('light', 'standard', 'deep'));
```
`migrations/000006_scans_preset.down.sql`:
```sql
ALTER TABLE scans DROP COLUMN preset;
```

- [ ] **Step 2: Apply migration to a throwaway/dev DB and verify**

Run（DB が 127.0.0.1:5432 で起動している前提。未起動なら `make db-up`）:
```bash
migrate -path migrations -database "$DATABASE_URL" up
migrate -path migrations -database "$DATABASE_URL" down 1
migrate -path migrations -database "$DATABASE_URL" up
```
Expected: up/down/up がエラーなく完了

- [ ] **Step 3: Update CreateScan query (api)**

`api/internal/db/queries/scans.sql` の `CreateScan` を置換:
```sql
-- name: CreateScan :one
INSERT INTO scans (site_id, preset)
VALUES ($1, $2)
RETURNING *;
```
> worker 側 `queries/scans.sql` の CreateScan は存在しない（worker は作成しない）ため変更不要。ただし worker の sqlc は `RETURNING *` 経由で `Scan` struct に `Preset` フィールドが増えるため再生成が必要。

- [ ] **Step 4: Regenerate sqlc (both modules)**

Run:
```bash
cd api && sqlc generate && cd ..
cd worker && sqlc generate && cd ..
```
Expected: `CreateScanParams` に `Preset string` が追加され、`db.Scan` struct に `Preset` が増える

- [ ] **Step 5: Verify build (both modules compile with new params)**

Run: `cd api && go build ./... ; cd ../worker && go build ./...`
Expected: api は `q.CreateScan(ctx, siteID)` 呼び出し箇所（scan/service.go）が引数不足でFAIL するはず → Task 10 で修正。worker は成功。

> この時点で api がビルドエラーになるのは想定内（Task 10 で service を直す）。migration + 生成物のみコミットする。

- [ ] **Step 6: Commit**

```bash
git add migrations/000006_scans_preset.up.sql migrations/000006_scans_preset.down.sql \
  api/internal/db/queries/scans.sql api/internal/db/ worker/internal/db/
git commit -m "db: scans に preset カラムを追加し sqlc を再生成する"
```

---

### Task 10: api `scan.Service.EnqueueScan` を preset 対応に

サービス層で preset を検証・保存・ジョブ引数へ載せる。

**Files:**
- Modify: `api/internal/scan/service.go`
- Modify: `api/internal/scan/service_integration_test.go`

**Interfaces:**
- Consumes: `jobs.ParsePreset`/`jobs.ScanArgs`（Task 1/3）, `db.CreateScanParams{SiteID, Preset}`（Task 9）
- Produces: `func (s *Service) EnqueueScan(ctx, siteID uuid.UUID, preset jobs.Preset) (uuid.UUID, error)`; `var ErrInvalidPreset`（`jobs.ErrInvalidPreset` を再エクスポートせず、handler は jobs.ErrInvalidPreset を直接判定）

- [ ] **Step 1: Update integration test call sites**

`api/internal/scan/service_integration_test.go` の `EnqueueScan(ctx, siteID)` 呼び出しを `EnqueueScan(ctx, siteID, jobs.PresetStandard)` に更新する（import に `jobs` を追加）。無効 preset の検証は handler 層（Task 11）で行うため、ここでは有効値で既存経路を維持する。

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags=integration ./internal/scan/ -run TestEnqueue -v` （TEST_DATABASE_URL 設定済み前提。未設定ならビルドのみ `go vet -tags=integration ./internal/scan/`）
Expected: FAIL / コンパイルエラー（引数不一致）

- [ ] **Step 3: Modify service**

`api/internal/scan/service.go`:
- import に `"github.com/ymd38/goodast/jobs"` は既にある。
- `EnqueueScan` シグネチャと本体を更新:
```go
func (s *Service) EnqueueScan(ctx context.Context, siteID uuid.UUID, preset jobs.Preset) (uuid.UUID, error) {
	if _, err := jobs.ParsePreset(string(preset)); err != nil {
		return uuid.Nil, err // jobs.ErrInvalidPreset。handler が 400 に翻訳する。
	}
	tx, err := s.pool.Begin(ctx)
	// ...（所有確認までは既存のまま）...

	scan, err := q.CreateScan(ctx, db.CreateScanParams{
		SiteID: pgtype.UUID{Bytes: siteID, Valid: true},
		Preset: string(preset),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create scan: %w", err)
	}

	scanID := uuid.UUID(scan.ID.Bytes)
	if _, err := s.river.InsertTx(ctx, tx, jobs.ScanArgs{ScanID: scanID.String(), Preset: preset}, nil); err != nil {
		return uuid.Nil, fmt.Errorf("enqueue scan job: %w", err)
	}
	// ...（commit は既存のまま）
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test -tags=integration ./internal/scan/ -race`（DB 必要）
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/scan/service.go api/internal/scan/service_integration_test.go
git commit -m "feat(scan): EnqueueScan で preset を検証・保存・ジョブ引数へ載せる"
```

---

### Task 11: handler `POST /scans` に preset を受け付ける

HTTP 層で preset をバインドし、不正値を 400 に翻訳する。

**Files:**
- Modify: `api/internal/handler/scan.go`

**Interfaces:**
- Consumes: `scan.Service.EnqueueScan(ctx, siteID, preset)`（Task 10）, `jobs.ErrInvalidPreset`/`jobs.Preset`（Task 1）

- [ ] **Step 1: Add preset to request + parse + error mapping**

`api/internal/handler/scan.go`:
- import に `"github.com/ymd38/goodast/jobs"` を追加。
- `startScanRequest` に preset を追加:
```go
type startScanRequest struct {
	SiteID string `json:"site_id" binding:"required"`
	Preset string `json:"preset"` // 省略可。空なら standard（jobs.ParsePreset）。
}
```
- `startScanResponse` に preset を追加（実際に使われた preset を返す）:
```go
type startScanResponse struct {
	ScanID string `json:"scan_id"`
	Status string `json:"status"`
	Preset string `json:"preset"`
}
```
- `start` 本体の enqueue 呼び出しを更新:
```go
	preset, err := jobs.ParsePreset(req.Preset)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preset"})
		return
	}

	scanID, err := h.svc.EnqueueScan(c.Request.Context(), siteID, preset)
	if err != nil {
		h.writeScanError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, startScanResponse{ScanID: scanID.String(), Status: "queued", Preset: preset.String()})
```
- `writeScanError` の switch に、service が万一 `jobs.ErrInvalidPreset` を返した場合の 400 分岐を追加（defense-in-depth。通常は上の ParsePreset で弾く）:
```go
	case errors.Is(err, jobs.ErrInvalidPreset):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
```
- swagger 注釈（@Description）に preset の説明を追記: `preset（light/standard/deep・省略時 standard）`。

- [ ] **Step 2: Verify build + existing handler tests**

Run: `cd api && go build ./... && go test ./internal/handler/ -race`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add api/internal/handler/scan.go
git commit -m "feat(scan): POST /scans で preset を受け付け不正値を 400 にする"
```

---

### Task 12: api `report` を共有 `jobs.ScanSummary` 経由デコードへ

ドリフト対策の api 側。`decodeSummaryCounts` を共有型経由にし、`report.SeverityCounts` へ変換する。

**Files:**
- Modify: `api/internal/report/repository.go`
- Modify: `api/internal/report/score.go`（コメントのみ）
- Modify: `api/internal/report/repository_test.go`（フィクスチャ確認）

**Interfaces:**
- Consumes: `jobs.ScanSummary`（Task 2）
- Produces: `decodeSummaryCounts(raw []byte) (SeverityCounts, error)`（内部で jobs.ScanSummary 経由）

- [ ] **Step 1: Rewrite decodeSummaryCounts to go through jobs.ScanSummary**

`api/internal/report/repository.go`:
- import に `"github.com/ymd38/goodast/jobs"` を追加。
- `decodeSummaryCounts` を置換:
```go
// decodeSummaryCounts は summary_json から重大度カウントを取り出す。worker が書く形は
// 共有型 jobs.ScanSummary（{"findings":{...}}）で固定されており、ここでそれを経由して
// api 固有の SeverityCounts（スコア計算メソッドを持つ）へ変換する。両側が同一の共有型を
// 経由するため、形の独立定義によるドリフトが構造的に起きない。
func decodeSummaryCounts(raw []byte) (SeverityCounts, error) {
	var summary jobs.ScanSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return SeverityCounts{}, err
	}
	return SeverityCounts(summary.Findings), nil
}
```
> `SeverityCounts(summary.Findings)` は `report.SeverityCounts` と `jobs.SeverityCounts` がフィールド名・型・順序・タグとも同一なため Go の型変換で成立する。相違があればコンパイルエラーになり、それ自体がドリフト検出になる。

- [ ] **Step 2: Update score.go comment**

`api/internal/report/score.go` の `SeverityCounts` 上のコメント（30-31 行）を更新:
```go
// SeverityCounts は 1 スキャンの重大度別 finding 件数（スコア計算用・api 固有）。
// フィールドは共有 wire 型 jobs.SeverityCounts と一致させ、repository が型変換で橋渡しする。
```

- [ ] **Step 3: Verify repository_test fixtures still match**

`api/internal/report/repository_test.go` を確認し、summary_json フィクスチャが `{"findings":{...}}` のネスト形であることを確かめる（既に PR #21 でネスト化済みのはず）。フラット形が残っていれば `{"findings":{...}}` に修正する。

- [ ] **Step 4: Run report tests**

Run: `cd api && go test ./internal/report/ -race -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/report/repository.go api/internal/report/score.go api/internal/report/repository_test.go
git commit -m "refactor(report): summary_json を共有型 jobs.ScanSummary 経由でデコードする"
```

---

### Task 13: integration テスト（nuclei）を Profile 明示構築へ更新

engine.ScanRequest の Profile 必須化に合わせ、SDK 統合テストを更新する。parity は決定性維持。

**Files:**
- Modify: `worker/internal/engine/nuclei/nuclei_integration_test.go`
- Modify: `worker/internal/engine/nuclei/auth_integration_test.go`
- Modify: `worker/internal/engine/nuclei/parity_integration_test.go`
- Modify: `worker/internal/engine/nuclei/redirect_leak_integration_test.go`（存在すれば）
- Modify: `worker/internal/scanjob/worker_integration_test.go`

**Interfaces:**
- Consumes: `engine.PlanFor`（Task 4）, `nuclei.New()`（Task 7）

- [ ] **Step 1: Update nuclei engine construction and ScanRequest**

各 nuclei integration テストで:
- `nuclei.New(nuclei.DefaultConfig())` を `nuclei.New()` に置換。
- `engine.ScanRequest{Scope: ..., Headers: ...}` に `Profile: engine.PlanFor(jobs.PresetLight).Scan` を追加（テスト実行を速くするため light を既定に。import に `jobs` 追加）。
- parity テストは、baseline CLI に渡すタグと `Profile.Tags` を**同一集合**にする。既存が `misconfig,tech` を前提にしているなら、`Profile` を明示的に `engine.ScanProfile{Tags: []string{"misconfig","tech"}, ExcludeTags: []string{"dos","intrusive"}, RateLimit:10, RatePeriod:time.Second}` で組み、CLI 側 baseline のタグも `misconfig,tech` に合わせる（`NUCLEI_TEST_TAGS` の既定と一致）。プリセット定義（light は exposure を含む）と baseline がズレないよう、parity は preset ではなく明示 Profile を使うことを優先する。

- [ ] **Step 2: Update scanjob worker integration test**

`worker/internal/scanjob/worker_integration_test.go`:
- `jobs.ScanArgs{ScanID: ...}` を作る箇所に `Preset: jobs.PresetLight`（または対象に応じた値）を追加。
- CreateScan を直接呼ぶヘルパがあれば `db.CreateScanParams{SiteID:..., Preset:"light"}` に更新。
- summary_json を検証する箇所は `jobs.ScanSummary` でデコードする形に更新（`scanSummary` は削除済み）。

- [ ] **Step 3: Verify integration build (compile only if no DB/templates)**

Run: `cd worker && go vet -tags=integration ./...`
Expected: コンパイル成功（実行はテンプレート/DB 環境が要るため任意）

- [ ] **Step 4: Commit**

```bash
git add worker/internal/engine/nuclei/*_test.go worker/internal/scanjob/worker_integration_test.go
git commit -m "test(engine): integration テストを preset Profile 明示構築へ更新する"
```

---

### Task 14: 全体検証 + PROGRESS.md 更新

全モジュールのテスト・lint を通し、進行管理を最新化する。

**Files:**
- Modify: `PROGRESS.md`

- [ ] **Step 1: Run full unit test + lint (all modules)**

Run:
```bash
cd jobs && go test ./... -race && golangci-lint run && cd ..
cd worker && go test ./... -race && golangci-lint run && cd ..
cd api && go test ./... -race && golangci-lint run && cd ..
```
Expected: 全 PASS、lint 0 issues

- [ ] **Step 2: Verify coverage of pure packages (jobs / engine)**

Run:
```bash
cd worker && go test -covermode=atomic -coverprofile=/tmp/cov.out ./internal/engine/ && go tool cover -func=/tmp/cov.out | grep -E "preset.go|summary.go"
cd ../jobs && go test -covermode=atomic -coverprofile=/tmp/covj.out ./... && go tool cover -func=/tmp/covj.out | grep -E "^total"
```
Expected: preset.go / summary.go / jobs 100%

- [ ] **Step 3: Update PROGRESS.md**

`PROGRESS.md` の「手動 E2E で判明した課題」表の ① 行を `✅ 本修正済み` に更新し、①の正式対応セクション・「直近のアクション」の次タスク候補①を完了として書き換える。②のドリフト再発防止（共有型化）も完了として追記する。「現在地スナップショット」に本 PR の完了を1行追加する。

- [ ] **Step 4: Commit**

```bash
git add PROGRESS.md
git commit -m "docs: スキャンプリセット正式対応と summary_json ドリフト対策の完了を記録する"
```

- [ ] **Step 5: (任意) 実走確認**

DB + テンプレート導入環境があれば、`make db-up && make migrate` 後に api/worker を起動し、`POST /scans {site_id, preset:"light"}` で 202 が返り、worker が light プリセットで完走、ダッシュボードにカウントが反映されることを手動確認する。この手動 E2E は環境依存のため必須ステップではないが、実施した場合は結果を PROGRESS.md に追記する。

---

## Self-Review

**Spec coverage:**
- プリセット識別子 jobs/ → Task 1 ✅
- summary_json 共有型 jobs/ → Task 2, 5, 8, 12 ✅
- ScanArgs に preset（Timeout callback 対応）→ Task 3, 8 ✅
- engine PlanFor（tags/rate/timeout 写像）→ Task 4 ✅
- ScanRequest.Profile → Task 6, 7, 8 ✅
- nuclei ハードコード config 撤廃 → Task 7 ✅
- scans.preset カラム + migration + sqlc → Task 9 ✅
- api service/handler preset 受付・検証（400）→ Task 10, 11 ✅
- report デコード共有型経由 → Task 12 ✅
- integration テスト更新（parity 決定性維持）→ Task 13 ✅
- 検証 + PROGRESS 更新 → Task 14 ✅

**Placeholder scan:** プレースホルダなし。各コード step は実コードを提示。

**Type consistency:**
- `jobs.Preset` / `PresetLight/Standard/Deep` / `DefaultPreset` / `ParsePreset` / `ErrInvalidPreset` — Task 1 定義、Task 3/4/8/10/11 で一貫使用。
- `jobs.SeverityCounts` / `jobs.ScanSummary` — Task 2 定義、Task 5/8/12 で一貫使用。
- `engine.ScanProfile` / `engine.Plan` / `engine.PlanFor` — Task 4 定義、Task 6/7/8/13 で一貫使用。
- `engine.ScanRequest{Scope, Headers, Profile}` — Task 6 定義、Task 7/8/13 で一貫使用。
- `nuclei.New()`（引数なし）— Task 7 定義、Task 13 で一貫使用。
- `EnqueueScan(ctx, siteID, preset)` — Task 10 定義、Task 11 で一貫使用。
- `db.CreateScanParams{SiteID, Preset}` — Task 9 生成、Task 10 で使用。

**ビルド順序の注意:** Task 5→6→7→8 は worker のビルドが途中で壊れ得る（engine.Summary 削除・cfg 撤廃）。Task 8 完了時点で worker 全体がビルド・テスト可能になる。Task 9→10 は api が途中で壊れ得る（CreateScan 引数変更）。Task 11 完了時点で api がビルド可能。subagent-driven で1タスクずつ回す場合、各タスク末尾の commit は「そのタスクのファイル群」に閉じるが、モジュール全体のビルド確認は Task 8（worker）/ Task 11（api）で行う。
