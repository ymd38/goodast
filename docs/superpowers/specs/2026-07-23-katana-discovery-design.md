# Katana 探索（Discovery）機能 設計 — Phase 2 探索コア

> ステータス: **設計 spec（実装なし）**。spec #30（`2026-07-08-crawl-guardrail-policy-design.md`）が
> 「別 ADR に先送り」としたクロールエンジン選定と、その最初の実装スコープを確定する。
> 本 spec 承認後に writing-plans で実装プランへ落とす。

作成: 2026-07-23
改訂: 2026-07-23（research 反映）— Katana SDK は custom `http.RoundTripper` を受けないため、
spec #30 が描いた「ScopedTransport（RoundTripper）を crawler/nuclei で共有」は不採用。クロール段の
リクエスト時遡断は **Katana native scope への写像 ＋ `OnResult` 後段チェック ＋ standard engine の
GET-only** で実現する（§3）。セキュリティ保証（逸脱が物理的に起きない）は維持される。

## 0. この spec の位置づけ

spec #30 は「クロール段（GET-only＋フォーム抽出）→ スキャン段（診断）」の二段分離と、
リクエスト時スコープ遡断、そして**エンジン非依存**の `Crawler` 枠組みを方針として定めた。
ただし **クロールエンジンの選定（katana / ZAP）** と具体的な実装スコープ、および遡断の**実装手段**は
先送りにしていた。本 spec はそれを次のように確定する:

- **エンジン: Katana の Go SDK**（CLI wrapper ではない）。**ZAP は未確定オプション**（採用前提にしない）。
- **今回のスコープ: 探索コアのみ**。発見した GET 到達 URL 群を既存 Nuclei 診断に流し、
  診断対象を「単一 URL」から「発見 URL 群」へ拡張する。**フォームファジングは次イテレーション**。
- **遡断の実装手段: Katana native scope ＋ `OnResult` 後段チェック**（RoundTripper 注入は Katana 非対応）。

## 1. 決定サマリ（先に結論）

| 論点 | 決定 |
|---|---|
| クロールエンジン | **Katana Go SDK**（`worker/internal/engine/discovery/katana`。独立 `Crawler` 実装） |
| ZAP | **未確定オプション**（採用を前提にしない）。Katana+Nuclei（＋将来 Nuclei DAST fuzzing）で当面の DAST スコープを賄う |
| クロール方式 | **standard クローラのみ**（net/http ベース・JS 実行なし）。headless は将来 preset |
| クロール段の method | **GET のみ**。standard engine は既定でフォーム送信しない（`AutomaticFormFill` は headless 専用・無効のまま） |
| フォームの扱い（今回） | `FormExtraction=true` で抽出し **件数のみ集計**（`ScanSummary` に載せる）。詳細の永続化はテーブルと共に**次タスク** |
| スキャン段 | 発見した **GET 到達 URL 群** を既存 Nuclei で診断（テンプレ由来 POST は従来通り含む） |
| フォームファジング（②） | **今回やらない**（次イテレーション） |
| クロール段ガード | **Katana native scope（`FieldScope=fqdn` ＋ `OutOfScope`＝危険パス regex）＋ `OnResult` の `scope.Allows` 後段チェック ＋ standard engine（GET-only）**。RoundTripper 注入は Katana 非対応のため不採用 |
| Nuclei 段ガード | 従来の post-filter（`scope.Allows`）維持。渡す URL は Katana スコープ内で発見済み＝構造的に in-scope |
| Nuclei への変更 | **加算のみ**（`ScanRequest.Targets` を足し `LoadTargets` の入力を差し替え）。ガード/SDKオプション/認証注入/W3 は不変 |
| ワークロード/時間 | preset 別クロール上限（depth / max URL）＋ 動的タイムアウト（発見 URL 数連動）＋ preset 別 CEILING |
| 永続化（今回） | 発見 URL 数・フォーム数を `jobs.ScanSummary.Discovery` に追加（omitempty）。`discovered_endpoints` テーブルは**次タスク** |
| ダッシュボード UI | **別セッション（frontend）**。今回は backend が summary に counts を出すところまで |

## 2. スコープ（今回やる / やらない）

### 今回やる（backend セッション）
- Katana SDK を独立 `Crawler` として `worker/internal/engine/discovery/katana` に隔離。
- `engine` 純粋層に `Crawler` interface・`CrawlPlan`・動的タイムアウト式・`TargetsOrBase` を追加。
- `scanjob/worker.go` を **crawl段 → 動的タイムアウト算出 → scan段** の二段に配線。
- 発見 GET URL 群を `ScanRequest.Targets` で Nuclei に渡し診断（対象拡張）。
- preset にクロール上限を追加（light=無効 / standard=浅い / deep=広い）。
- 発見 URL 数・フォーム数を `jobs.ScanSummary` に追加。
- Juice Shop での integration 検証（探索で対象拡張・危険パス不踏・スコープ逸脱なし・上限が効く）。

### 今回やらない（次イテレーション以降）
- **フォームファジング**（spec #30 スキャン段②）。フォームは件数集計まで。
- **`discovered_endpoints` テーブル＋ `GET /scans/:id/endpoints` API**（発見エンドポイント一覧）。
- **発見エンドポイント一覧 UI・探索進捗の可視化**（frontend 別セッション）。
- **Nuclei への per-request scope 注入**（ADR-0002 の独立持ち越し・§3）。
- **headless（Chromium）クロール**。
- robots.txt / 外部対象向け rate 尊重ポリシー。

## 3. クロール段ガード（Katana native scope への写像）

spec #30 §3.1 は「リクエスト時遡断（逸脱が物理的に起きない）」を要求する。research の結果、
**Katana SDK は custom `http.RoundTripper`/`http.Client` を受け付けない**（`types.Options` は
`Proxy`/`CustomHeaders`/`Resolvers` を持つが transport 注入口が無い）。Nuclei も同様（ADR-0002）。
よって「共有 ScopedTransport（RoundTripper）」は**どちらのエンジンにも刺さらず、消費者ゼロのデッドコード**
になるため**採用しない**。代わりに、Katana が持つ**リクエスト前スコープ判定**へガードを写像する。

### 3.1 クロール段（Katana）の遡断機構
`engine.Scope`（allowlist・危険パスの唯一の正）を Katana オプションへ写像する:

- **host allowlist**: `FieldScope="fqdn"`（seed の完全一致ホストに限定・サブドメイン追わない）。
  ポート厳密判定は Katana scope では表現しきれないため、`OnResult` で `scope.Allows(url)`
  （同一 host:port＋非危険パス）を後段チェックし、不一致は破棄する（＝収集しない）。
- **危険パス遡断**: 危険セグメント（`logout`/`signout`/`delete`/`remove`/`destroy`/`admin`）を
  `OutOfScope` 正規表現として渡し、Katana に**辿らせない**。加えて `OnResult` の `scope.Allows`
  （内部で `IsDangerousPath`）が後段でも破棄する（belt-and-suspenders）。
- **GET-only / フォーム不送信**: `standard.New` エンジンを使う。standard は GET でナビゲートし、
  **フォームを送信しない**（`AutomaticFormFill` は headless 専用機能で無効のまま）。`FormExtraction=true`
  でフォームを構造的に抽出する（送信はしない）。
- **クロール上限**: `MaxDepth`（preset 別）＋ **発見 URL 数の上限**（`OnResult` で受理数を数え、上限到達で
  クロール `context` を cancel → Katana が graceful に停止）。
- **cross-host redirect**: native scope が別ホストへの追従を抑止。`OnResult` 後段チェックが
  authority 不一致の結果を破棄する二重防御。

> **リクエスト時遡断は維持される**: Katana は out-of-scope / OutOfScope の URL を**辿らない（fetch しない）**
> ため、逸脱は crawl-follow の段階で物理的に起きない。`engine.Scope` を単一の正として Katana 設定へ
> 写像することで、allowlist・危険パス定義の一元管理（spec #30 §6）も保つ。

### 3.2 Nuclei 段の割り切り（ADR-0002 持ち越し）
Nuclei SDK v3.9.0 も per-request の host/path allowlist を安全に注入できない（`WithOptions` が既定を
丸ごと置換する）。よって **Nuclei 段は従来どおり post-filter（`scope.Allows`）を維持**する。Nuclei に
渡す対象 URL は Katana のスコープ内で発見されたものだけなので**構造的に in-scope**であり、post-filter は
defense-in-depth として残る。「Nuclei への per-request scope 注入」は本 spec のスコープ外（ADR-0002 の
独立課題）。

## 4. アーキテクチャ: パッケージ配置

既存 `nuclei/` 構造に対称。純粋層は `engine` に、SDK アダプタは配下パッケージに隔離する。

```
worker/internal/engine/
  engine.go          既存 + Crawler interface + ScanRequest.Targets を追加
  crawl.go           ★新規 CrawlPlan / ScanTimeout / TargetsOrBase / dangerousPathRegexes（純粋・unit 100%）
  preset.go          既存 Plan に Crawl CrawlPlan を追加
  scope.go           既存（allowlist・危険パスの唯一の正。Katana 写像と後段チェックが再利用）
  discovery/
    katana/
      katana.go                  ★新規 Katana SDK アダプタ（engine.Crawler 実装）
      katana_integration_test.go ★新規 //go:build integration（coverage 除外）
  nuclei/
    nuclei.go        既存（LoadTargets の入力を engine.TargetsOrBase(req) に差し替え・ガードは不変）
  scanjob/
    worker.go        crawl段→動的timeout→scan段の二段配線（Crawler を dig 注入）
```

- `Crawler` interface は純粋 `engine` パッケージに置く（既存 `Engine` interface と同じ扱い）。
  **正当化はテスト容易性**：二段オーケストレーション（`scanjob`）を実クロールなしで検証するため、
  crawler を fake に差し替えられる境界が要る。ZAP は未確定オプションのため interface の主根拠にはしない
  （ZAP が確定すればそのまま第2実装として乗る副次利点はある）。
- `discovery/katana` は nuclei アダプタと同様 `//go:build integration` で検証し、ユニット coverage 計測から
  除外する（SDK 呼び出しはネットワーク＋クロール対象を要するため）。純粋ロジック（上限判定・タイムアウト式・
  scope 写像 regex）は親 `engine` に置き 100% ユニット網羅する。

### 4.1 型と interface（案）

```go
// engine パッケージ（純粋）
type CrawlPlan struct {
    Enabled  bool
    MaxDepth int
    MaxURLs  int
}

// CrawlResult はクロール段の成果。URLs は GET 到達済み・スコープ内の対象（重複排除済み）。
// FormCount は抽出フォーム数（今回は件数のみ・詳細永続化は次タスク）。
type CrawlResult struct {
    URLs      []string
    FormCount int
}

// Crawler はクロールエンジンの抽象（Engine と同じ扱い・実装は discovery 配下に隔離）。
type Crawler interface {
    // Crawl は scope 起点から plan の上限内で GET 探索し、発見 URL とフォーム数を返す。
    // headers は認証クロール用の "Name: Value"（ADR-0003・未認証時は空）。
    Crawl(ctx context.Context, scope Scope, plan CrawlPlan, headers []string) (CrawlResult, error)
    Version() string // 例 "katana/vX.Y.Z"（scans への記録に用いる）
}
```

## 5. 二段オーケストレーション & データフロー

`scanjob/worker.go` の `runScan` を二段化する（spec #30 §3 の順序に従う）:

```
target/scope/headers ロード（既存のまま）
  │
  ▼ crawl段
  plan := engine.PlanFor(preset)                       // Crawl CrawlPlan を含む（§6）
  urls := []string{scope.BaseURL()}                    // 既定は単一 URL（現状維持）
  formCount := 0
  if plan.Crawl.Enabled {
      res, err := w.crawler.Crawl(ctx, scope, plan.Crawl, headers)
      // err は既存のエラー分類に乗せる（§7）。res.URLs が空ならフォールバックで単一 URL。
      if err == nil && len(res.URLs) > 0 { urls = res.URLs; formCount = res.FormCount }
  }
  │
  ▼ 動的タイムアウト
  ctx = context.WithTimeout(ctx, engine.ScanTimeout(len(urls), plan.Timeout))  // plan.Timeout=CEILING
  │
  ▼ scan段（既存）
  findings := w.engine.Scan(ScanRequest{Scope, Targets: urls, Headers, Profile})
  │
  ▼ 保存
  summary = ScanSummary{ Findings, Discovery: &DiscoveryInfo{ URLCount: len(urls), FormCount: formCount } }
```

### 5.1 Nuclei への最小・加算変更
- `engine.ScanRequest` に `Targets []string` を追加。
- `engine.TargetsOrBase(req)` 純粋関数（`req.Targets` が空なら `[]string{req.Scope.BaseURL()}`、
  非空ならそのまま）を追加し unit 網羅。
- `nuclei.go` の `ne.LoadTargets([]string{req.Scope.BaseURL()}, false)` を
  `ne.LoadTargets(engine.TargetsOrBase(req), false)` に変更するのみ。
- **ガードレール・SDK オプション・認証注入（ADR-0003）・W3（DisableRedirects）・post-filter は一切変更しない。**
  対象の供給元が「単一 scope URL」→「発見 URL 群」に変わるだけ。既存挙動は Targets 空フォールバックで後方互換。

### 5.2 `jobs.ScanSummary` の拡張
```go
type ScanSummary struct {
    Findings  SeverityCounts `json:"findings"`
    Discovery *DiscoveryInfo `json:"discovery,omitempty"` // クロール無効時 nil
}
type DiscoveryInfo struct {
    URLCount  int `json:"url_count"`
    FormCount int `json:"form_count"`
}
```
- `Discovery` は omitempty（light や単一 URL スキャンでは nil）。既存の summary デコード（api `report`）は
  `Findings` のみ参照のため後方互換。api 側の表示追加は別途（frontend セッション）。

### 5.3 DI（dig）
- `scanjob.WorkerDeps` に `Crawler engine.Crawler` を追加、`NewWorker` で保持。
- `worker/cmd/worker/main.go` で `katana.New()` を `container.Provide`（`engine.Crawler` として）。

## 6. preset とクロール有界化・動的タイムアウト

`engine.Plan` に `Crawl CrawlPlan` を追加する。`PlanFor` の写像（初期値・調整可能）:

| preset | クロール | 上限 | CEILING（既存 Timeout 流用） |
|---|---|---|---|
| light | 無効（単一 URL・現状維持） | — | 15 分 |
| standard | 浅い | depth 2 / max 50 URL | 30 分 |
| deep | 広い | depth 3 / max 200 URL | 60 分 |

### 6.1 動的タイムアウト（純粋関数 `engine.ScanTimeout`）
発見 URL 数に比例した時間枠を CEILING でキャップする。spec #30 §5.2 の式は `|templates|` を含むが、
テンプレ数は実行時に安価に得られないため、**URL 数あたりの時間予算に畳んだ実装可能形**に単純化する:

```
ScanTimeout(numURLs, ceiling) = clamp(base + numURLs × perURLBudget, floor, ceiling)
  base         = 2 分   （単一 URL でも確保する下駄）
  perURLBudget = 10 秒  （発見 URL 1 本あたりの追加枠）
  floor        = 2 分
  ceiling      = preset 別（= PlanFor(preset).Timeout）
```

- **river ジョブ Timeout = preset の CEILING**（既存 `Worker.Timeout` = `PlanFor(preset).Timeout`）。
  river callback は URL 数を持てないため、クロール後に**内側で** `context.WithTimeout(ctx, ScanTimeout(...))`
  を適用。ジョブは 1 本のまま二段を実行する。
- `base` / `perURLBudget` / `floor` の値は実測でチューニング（spec #30 §9）。純粋関数なので境界を unit 網羅。

## 7. エラーハンドリング

- **クロール失敗**: 既存の runScan エラー分類に乗せる。一過性（DB/一時ネットワーク）は river 再試行、
  最終試行のみ failed。`context.DeadlineExceeded` は既存どおり即 failed（同 preset 再試行で解消しないため）。
  クロール自体の失敗は**単一 URL フォールバックで診断続行**とし、探索不能でスキャン全体を落とさない。
- **クロール 0 件**（リンク未発見・全て out-of-scope 等）: 失敗ではなく **単一 URL（scope.BaseURL）へフォールバック**。
  空スキャンにしない。
- **クロール中の危険パス/スコープ外**: Katana が辿らず（＋ `OnResult` 後段破棄）、そもそも収集しない。
- 認証クロール: 既存 ADR-0003 のヘッダを `headers` で Katana `CustomHeaders` に渡す。GET-only＋危険パス遡断で
  認証状態でも状態を変えない（spec #30 §4）。値は一切ログしない。

## 8. テスト方針

### 純粋 unit（100% 網羅・ネットワーク不要）
- **`ScanTimeout`**: clamp の floor / ceiling 境界、URL=0 / 大規模の両端。
- **`CrawlPlan` 写像（`PlanFor`）**: light=無効 / standard / deep の depth・maxURL をテーブル駆動で網羅。
- **`TargetsOrBase`**: Targets 空→base、非空→そのまま。
- **`dangerousPathRegexes`**（Katana `OutOfScope` 用 regex 生成）: 危険セグメントを網羅する regex を返すことを検証。
- スコープ判定（host:port＋危険パス）は既存 `scope_test.go`（`Allows` / `IsDangerousPath`）で網羅済みを流用。

### integration（`//go:build integration`・coverage 除外）
- **Katana アダプタ**（`discovery/katana`）: Juice Shop を実クロールし、
  1. **探索で診断対象が拡張される**（発見 URL 集合が単一 URL の上位集合＝ hard assert）。finding 件数は
     Juice Shop がステートフルで非決定的なため（既存 parity テストと同じ理由）、「探索ありは単一 URL 時以上」を
     下限として assert し、件数差はレポートに留める。
  2. **危険パスを踏まない**（logout/delete/admin 等が発見 URL 集合に含まれない）。
  3. **allowlist 外へ出ない**（別ホスト authority が発見 URL 集合に含まれない）。
  4. クロール上限（`MaxDepth` / `MaxURLs`）が効く（`MaxURLs` を小さくすると発見数が頭打ち）。
- `make discovery-scan` 追加（`NUCLEI_TEST_TARGET` 等の既存パターンに合わせる）。

## 9. 依存・配布

- **Katana Go SDK**（`github.com/projectdiscovery/katana`）を worker モジュールに追加（`go.mod` 固定）。
  多くの projectdiscovery 系依存（utils / retryablehttp 等）は Nuclei 経由で既に推移依存にあり、増分は小さい。
  導入時に `go get` で解決した版を**固定**し、`Version()` に反映する。
- **CLI バイナリは同梱しない**（【決定 2026-07-02】と一致・SDK 静的リンク）。
- Katana バージョンは `go.mod` で固定し `go get -u` しない（ADR-0002 に準じる）。

## 10. 前方互換・非破壊の確認

- 既存の**単一 URL 診断は挙動不変**（`CrawlPlan.Enabled=false` の light、および `Targets` 空フォールバック）。
- `ScanSummary.Discovery` は omitempty で既存デコードに後方互換。
- `engine.Scope` を allowlist・危険パスの唯一の正として維持（Katana 写像・後段チェックが再利用）。
- Nuclei の parity（§10）・認証注入・W3 対策に影響を与えない（Targets 加算のみ）。

## 11. 未決定（writing-plans の research 段で詰める / 一部解決済み）

- **【解決】Katana は custom transport を受けない** → native scope 写像＋ `OnResult` 後段チェックで遡断（§3）。
- **【解決】動的タイムアウト式** → `|templates|` を用いず URL 数×時間予算に単純化（§6.1）。
- Katana SDK の細部（`FieldScope` の正確な列挙値・`OnResult` の `output.Result` からフォーム有無を判定する
  フィールド・`Version()` 取得方法）は plan の各タスクで実 SDK に当てて確定する。
- preset 別クロール上限・`base`/`perURLBudget`/`floor` の初期値の妥当性（実測調整）。
- integration の Juice Shop 期待値（発見 URL 数の目安・危険パスの具体セット）。
