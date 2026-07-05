# スキャンプリセット正式対応 + summary_json ドリフト対策 — 設計

最終更新: 2026-07-06

PROGRESS.md「手動 E2E で判明した課題」①の正式対応。暫定回避（tags ハードコード
`misconfig,tech` / timeout 固定 10 分・commit `7df0a53`）を、プリセット選択（軽量/標準/
詳細）と契約の一元化に置き換える。②の再発防止（summary_json ドリフト）も同一 PR で行う。

## 背景と課題

- `worker/internal/engine/nuclei/nuclei.go` の `DefaultConfig()` がタグ `misconfig,tech`
  をハードコード。プリセット選択（企画書 §6-2）が実現できていない。
- `scanjob.Worker.Timeout()` が 10 分固定（暫定）。プリセット別の適切値になっていない。
- `summary_json` の形を worker（`scanjob.scanSummary` = `{"findings":{...}}`）と
  api（`report.SeverityCounts`）が**別モジュールで独立定義**。過去に不一致でカウント 0 化
  のバグ（②）を出しており、再発リスクが残る。

## 決定事項（承認済み）

1. プリセットは **`scans.preset` カラム**に永続化（履歴・ダッシュボード記録用）。
2. summary_json ドリフト対策は **`jobs/` に共有型**を置く（コンパイル時に形を固定）。
3. プリセット + タイムアウト config 化 + ドリフト対策を **1 PR** にまとめる。
4. プリセット値（rate 全案 10req/s・`exclude dos,intrusive` 共通）:

   | preset | tags | timeout |
   |---|---|---|
   | light（軽量） | misconfig, tech, exposure | 5 分 |
   | standard（標準） | light + exposed-panels, default-login, cve | 15 分 |
   | deep（詳細） | standard + xss, sqli, lfi, ssrf, rce, takeover | 30 分 |

   deep でも全 13k テンプレは回さず、タグで有界化してタイムアウトを避ける。

## アーキテクチャ

### A. プリセット識別子は `jobs/`、マッピングは `engine`

ADR-0002 により **api は `worker/internal/engine` を import 不可**。プリセット識別子の
enum は api（検証・保存）と worker（Timeout・scan config）の双方が参照するため、
SDK 非依存の共有モジュール `jobs/` に置く。エンジン固有の値（タグ/レート/タイムアウト）
への写像は `engine`（`jobs` を import 可）に置き、unit 100% で網羅する。

```
jobs/preset.go
  type Preset string                    // "light" / "standard" / "deep"
  const PresetLight/PresetStandard/PresetDeep
  DefaultPreset = PresetStandard
  ParsePreset(string) (Preset, error)   // 不正値を拒否（HTTP 400 / DB CHECK と二重防御）

engine/preset.go   （純粋・unit 100%）
  type ScanProfile struct {
      Tags        []string
      ExcludeTags []string
      Severities  string
      RateLimit   int
      RatePeriod  time.Duration
  }
  type Plan struct {
      Scan    ScanProfile
      Timeout time.Duration
  }
  func PlanFor(p jobs.Preset) Plan       // preset → 実行パラメータ
```

### B. データフロー（preset の伝播）

`river` の `Timeout(job *river.Job[T]) time.Duration` は **context も DB も持てない**
callback。よって preset を **`jobs.ScanArgs` に載せて** enqueue 時に確定させる。加えて
**`scans.preset` カラム**にも永続化する（river ジョブ引数は完了後に消えるため、履歴・
ダッシュボードの記録は DB カラムが正）。

```
POST /scans { "site_id": ..., "preset": "standard" }   // preset 省略時は standard
  → handler: バインド
  → scan.Service.EnqueueScan(ctx, siteID, preset)
      - jobs.ParsePreset で検証（不正 → ErrInvalidPreset → 400）
      - q.CreateScan(site_id, preset)                 // scans.preset カラム
      - river.InsertTx(ScanArgs{ScanID, Preset})      // river ジョブ引数
  → 202 { scan_id, status: "queued" }

worker:
  Worker.Timeout(job)  → engine.PlanFor(jobs.Preset(job.Args.Preset)).Timeout   // DB 不要
  Worker.runScan       → plan := engine.PlanFor(preset)
                         engine.Scan(ctx, ScanRequest{Scope, Headers, Profile: plan.Scan}, cb)
```

`engine.ScanRequest` に `Profile ScanProfile` を追加。`nuclei.Scan` は `e.cfg` でなく
**`req.Profile`** を読む。これで per-scan にタグ/レートが可変になり、`nuclei.DefaultConfig()`
のハードコードを撤廃する（`nuclei.Config`/`DefaultConfig`/`Engine.cfg` は不要になり削除）。

worker の `Work()` は preset を `job.Args.Preset` から得る（`GetScanTarget` への追加は不要）。
`ParsePreset` 失敗時は標準にフォールバックせず設定不備として失敗扱い（api で検証済みのため
通常は起きないが、防御的に）。

### C. summary_json のドリフト対策（②の根本）

canonical な wire 契約を `jobs/` に一元化する。

```go
// jobs/summary.go
type ScanSummary struct {
    Findings SeverityCounts `json:"findings"`
}
type SeverityCounts struct {
    Critical int `json:"critical"`
    High     int `json:"high"`
    Medium   int `json:"medium"`
    Low      int `json:"low"`
    Info     int `json:"info"`
    Total    int `json:"total"`
}
```

- **worker**: `scanjob.scanSummary` を廃し `jobs.ScanSummary` を marshal。
  `engine.Summarize` は `jobs.SeverityCounts` を返す（engine は既に jobs を import）。
  `engine.Summary` は削除。
- **api**: `report.decodeSummaryCounts` を `jobs.ScanSummary` で unmarshal し、
  `report.SeverityCounts`（スコア計算メソッド `deduction()` を持つ api 固有型）へ変換。
  両側が `jobs.ScanSummary` を経由するため、**形の独立定義が消えてドリフトが構造的に固定**
  される。`report.SeverityCounts` はスコア振る舞いを持つため `jobs` へは寄せず残す。
- `jobs/` に round-trip 契約テスト（marshal → unmarshal で不変）を追加。

## DB マイグレーション

`migrations/000006_scans_preset.{up,down}.sql`

```sql
-- up
ALTER TABLE scans
  ADD COLUMN preset text NOT NULL DEFAULT 'standard'
  CHECK (preset IN ('light', 'standard', 'deep'));
-- down
ALTER TABLE scans DROP COLUMN preset;
```

`DEFAULT 'standard'` で既存行を安全に埋める。適用後、api / worker 両モジュールで
`sqlc generate` を再実行し `CreateScan` に preset 引数を追加する。

## 影響ファイル

| モジュール | 変更 |
|---|---|
| `jobs/` | `preset.go`（新規）/ `summary.go`（新規）/ `scanargs.go`（`Preset` 追加）/ 契約テスト |
| `engine` | `preset.go`（新規・PlanFor / ScanProfile / Plan）/ `engine.go`（ScanRequest.Profile 追加）/ `summary.go`（jobs.SeverityCounts 返却・engine.Summary 削除）+ unit |
| `engine/nuclei` | `nuclei.go`（req.Profile 参照・Config/DefaultConfig 撤廃）/ integration テスト（明示 Profile）|
| `scanjob` | `worker.go`（Timeout を PlanFor 化・runScan で Profile 配線・jobs.ScanSummary 使用）|
| `worker/cmd/worker` | `main.go`（`nuclei.New(nuclei.DefaultConfig())` → `nuclei.New()` 引数なしへ）|
| `api/scan` | `service.go`（EnqueueScan に preset 引数・ParsePreset 検証・ErrInvalidPreset）|
| `api/handler` | `scan.go`（preset バインド・ErrInvalidPreset → 400・swagger 注釈更新）|
| `api/report` | `repository.go`（decodeSummaryCounts を jobs.ScanSummary 経由に）/ `score.go`（SeverityCounts の由来コメント更新）|
| DB | `queries/scans.sql`（CreateScan に preset）両モジュール + sqlc 再生成 |
| migrations | `000006_scans_preset.{up,down}.sql` |

## テスト方針

- `jobs/`: `ParsePreset` テーブル駆動（全 preset + 不正値）、`ScanSummary` round-trip 契約。
- `engine`: `PlanFor` テーブル駆動（3 preset の Tags/Timeout/Rate を検証）unit 100%。
- `api/scan`・`handler`: preset 検証（有効/無効/省略時 standard）を結合・unit で網羅。
- integration（parity/auth）: `ScanRequest.Profile` を明示構築。parity は CLI ベースラインと
  同一タグを両側に適用して決定性を維持。
- `make test-api` / `make test-worker` パス、カバレッジゲート（unit 100%）維持。

## 非対象（YAGNI）

- レート・並列の env 化（プリセットで足りる。必要時に config へ）。
- deep の全テンプレ網羅（タイムアウトリスクが高くタグ有界で代替）。
- プリセットの UI・ウィザード（frontend 別セッション）。
