# PROGRESS — 進行管理

> セッションを跨いで作業を継続するための「現在地」メモ。
> **新しいセッションはまずこのファイルを読み、現在地・次アクションを把握する。**
> **各作業の区切りでこのファイルを更新する。** 決定の経緯は `MEMORY.md`、要件/フェーズは `docs/poc-plan.md` を正とする。

最終更新: 2026-07-04

---

## 現在地スナップショット

- フェーズ: **PoC Phase 1**
- 作業ブランチ: `main`（直近 PR #13 マージ済み・次タスク未着手）
- **PR #1〜#13 マージ済み**
- **ADR-0003 認証情報のアプリ層暗号化: 完結**（3段 PR #8〜#10）— #8 共有 `secrets/`（AES-256-GCM・AAD=siteID）/ #9 api 受付・暗号化保存（`PUT/DELETE/GET /sites/:id/credentials`）/ #10 worker 復号・`engine.ScanRequest.Headers`→nuclei `WithHeaders` 注入。これで**認証後スキャン**が api→worker で通る（詳細は下記ロードマップ）
- **§10-3 認証後スキャン検証: 完了**（#11）— `worker/internal/engine/nuclei/auth_integration_test.go`（`TestNucleiHeaderInjection` 決定的注入証明 / `TestNucleiAuthenticatedCoverage` カバレッジ縮小なし）+ `make nuclei-auth`。**残: ローカルで `make juiceshop-up` → `make nuclei-auth` 実走 PASS 確認**（nuclei-templates 導入環境が必要）
- **スコア計算: 完了**（#12）— `api/internal/report/score.go`（`Compute`/`Score`/`Band`/`Delta`・§5.1 の式・[0,100] クランプ・色は Band で frontend にマップ・unit 100%）
- **ダッシュボード集計 backend: 完了**（#13）— `api/internal/report/`（`dashboard.go` 純粋集計 / `repository.go` / `service.go`）+ `handler/dashboard.go`（`GET /sites/:id/dashboard`：最新スコア＋前回差分＋スコア時系列）+ sqlc `ListDoneScanSummaries`。**残: frontend（Chart.js 描画・別セッション）**
- **次タスク候補**: W3 ハードニング（クロスホスト redirect 認証ヘッダ漏えい・下記 backlog）/ findings 詳細レポート API / web (Nuxt) スキャフォールド（別セッション）
- sqlc: **v1.31.1** / river: **v0.39.0** / **Nuclei SDK: v3.9.0（go.mod 固定）**
- モジュール構成: api / worker / jobs / **secrets（認証情報暗号化・依存ゼロ・ADR-0003）** の4モジュール（go.work + replace）
- リモート: `ymd38/goodast`（**private**）
- ブランチ戦略: 2-tier（feature → main、PR経由）
- レビュー: **PR Agent（OpenAI）** に一本化

---

## ロードマップ（PoC Phase 1）

### 基盤
- [x] プロジェクトスキャフォールド（docs / ADR / .claude/rules / DESIGN / tokens.css）
- [x] ADR-0001 api/worker プロセス分離（go.work + 2モジュール）
- [x] Day-1 運用規約（slog / dig / config / pgxpool / graceful shutdown / health）
- [x] GitHub Actions（CI matrix / security-scan / PR Agent）
- [x] Makefile（`make help` で一覧。db/migrate/sqlc/dev/test/lint/cover/juiceshop/nuclei-scan）

### 実装
- [x] DBスキーマ: `migrations/000001_initial_schema` + sqlc セットアップ（企画書 §5）
  - 4テーブル（sites / scan_credentials / scans / findings）、text+CHECK、FK CASCADE
  - api/worker 両モジュールに sqlc.yaml + 生成コード（`internal/db/`）+ 最小クエリ
  - throwaway PG で migrate up/down 検証済み
- [x] ADR-0005 river ジョブキュー（api enqueue ↔ worker dequeue）
  - 共有 `jobs/` モジュール（ScanArgs / Kind="scan"）、river migrations 000002-000003
  - api: `EnqueueScan`（scan行作成 + river InsertTx を1txで atomic enqueue）+ insert-only client
  - worker: `ScanWorker`（queued→running→done のスタブ遷移。Nuclei は ADR-0002 で差込）+ graceful Stop
  - 結合テスト（//go:build integration）で enqueue→process→done / atomic enqueue を検証
- [x] ADR-0002 Nuclei SDK 統合（`worker/internal/engine/` の Work() に差込）
  - engine 純粋層（`engine.go` interface / `scope.go` allowlist・危険パス・所有確認 / `severity.go` 正規化 / `summary.go` 集計）= **unit 100%**
  - `engine/nuclei/` に Nuclei v3.9.0 SDK アダプタを隔離（scope filter・保守的レート・severity フィルタ・破壊的タグ除外・sandbox=ローカルファイル禁止）。`//go:build integration` で検証、coverage 除外
  - `GetScanTarget`（scans⨝sites）クエリ追加 + sqlc 再生成
  - `Work()` 差替: GetScanTarget → 所有確認 defense-in-depth（ADR-0004）→ engine.Scan → findings 保存 → summary_json → CompleteScan、設定不備は FailScan
  - 結合テスト（throwaway PG）で done+findings保存・resume冪等・未確認public→failed を検証
  - **未認証スキャンのみ**。session 認証（Cookie/Bearer 持込）の復号・注入は ADR-0003 へ分離
- [x] スキャン開始 HTTP エンドポイント（scan feature・EnqueueScan を呼ぶ）
  - `handler/scan.go`: `POST /scans`（`site_id` 受付 → EnqueueScan → **202 Accepted** + scan_id/status=queued）
  - エラー翻訳: 404（site 不在）/ **403**（所有確認未完了・ADR-0004 ガードレール。verify で解消可能な前提条件不足）/ 400（不正 uuid・body 欠落）/ 500（内部・ログ出力）
  - main.go DI 配線 + ルート登録。`writeScanError` は unit（テーブル駆動）、HTTP フローは結合テスト（throwaway PG + insert-only river client）で 202/403/404/400 全経路 + river_job 投入を検証
- [x] ADR-0004 ドメイン所有確認（ファイル設置 / DNS TXT）+ サイト登録 API
  - 共有 `api/internal/target/`（IsLocalTarget / RequiresOwnershipVerification）に集約し scan の私有コピーを DRY 化。unit 100%
  - `api/internal/site/`（types: VerifyMethod/VerifyToken、verify: Verifier〈file/DNS・注入可〉、repository、service）。純粋層 unit 100%
  - `api/internal/handler/site.go`: `POST /sites`（登録+トークン+設置ガイド）/ `GET /sites` / `GET /sites/:id` / `POST /sites/:id/verify`。ローカルは確認スキップ即 verified
  - main.go DI 配線 + ルート登録。HTTP フロー結合テスト（throwaway PG + fake verifier）で全経路検証
- [ ] ADR-0003 認証情報のアプリ層暗号化（`scan_credentials.enc_headers`）
- [x] スキャン受付 API 完成（scan 開始エンドポイント）※上記 `POST /scans` で完了。session 認証持込は ADR-0003 側
- [x] 検知精度 検証（§10）: Nuclei CLI ベースライン vs goodast の「欠落ゼロ」突合
  - `worker/internal/engine/nuclei/parity_integration_test.go`（`TestNucleiCLIParity`・`//go:build integration`）+ `make nuclei-parity`
  - CLI ベースラインに goodast の `DefaultConfig` と同一フィルタ（tags / exclude dos,intrusive / rate 10/s）を適用し、`scope.Allows` で絞った集合を正とする。判定は template-id 集合の包含（欠落ゼロ）で担保（URL 多重度・件数の完全一致はステートフル対象で非決定的なためレポートのみ）
  - **結果（Juice Shop @localhost:3001・tags=misconfig,tech・nuclei v3.9.0）: PASS。distinct template-id 一致（goodast=4 / baseline in-scope=4・欠落0）**。検出: `fingerprinthub-web-fingerprints` / `http-missing-security-headers` / `owasp-juice-shop-detect` / `tech-detect`。findings 件数差（goodast 4 / baseline 13）は同一テンプレの URL 多重度による
  - 認証後スキャン（§10-3）の検証も追加済み: `auth_integration_test.go`（`TestNucleiHeaderInjection` 決定的注入証明 + `TestNucleiAuthenticatedCoverage` カバレッジ縮小なし）/ `make nuclei-auth`。実走 PASS 確認は残（ローカル `make juiceshop-up`）
- [x] スコア計算（`api/internal/report/score.go`）※純粋ロジックのみ。ダッシュボード集計（DB/エンドポイント）は別項目
  - §5.1 の式 `max(0, 100 − (Critical×40 + High×10 + Medium×3 + Low×1))` を `Compute(SeverityCounts) Score` で実装（Info は減点なし・上下限 [0,100] を防御的にクランプ）
  - `Score` 値オブジェクト: `NewScore`（[0,100] 強制）/ `Value` / `Band`（good/caution/danger/crisis・境界 80/60/40）/ `Label`（良好/要注意/危険/危機）/ `Delta`（前回差分）
  - 色は backend で持たず **Band（セマンティック）を返し frontend が tokens.css の CSS 変数へマップ**（責務分離）
  - `SeverityCounts` の json タグは worker の `summary_json`（engine.Summary）と一致 → ダッシュボードが DB 値をそのままデコード可能
  - テーブル駆動テストで境界値・クランプ（負数カウント含む）・全バンド・Delta・NewScore 範囲外を網羅。**unit 100%**・lint 0 issues
- [ ] web (Nuxt) スキャフォールド → CI の frontend / pnpm-audit ジョブ有効化
- [~] ダッシュボード（スコア + 時系列・Chart.js）
  - **backend 集計 API 完了**（`api/internal/report/` + `handler/dashboard.go`）: `GET /sites/:id/dashboard`
    - sqlc `ListDoneScanSummaries`（done かつ summary_json あり を日付昇順）を追加
    - `dashboard.go`（純粋集計 `BuildDashboard`・unit 100%）: 最新スコア＋前回差分（初回は null）＋スコア時系列（history 昇順）。`Score`/`Band` を消費
    - `repository.go`（summary_json→SeverityCounts デコード境界・Date は finished_at 優先）/ `service.go`（gin 非依存）/ `handler/dashboard.go`（uuid 400 / スキャン無し=200＋latest:null・history:[]）
    - 結合テスト（throwaway PG）で 400・空・集計＋除外（queued / summary_json NULL）を検証。実 DB 実走 PASS
  - **残（frontend・別セッション）**: Chart.js の折れ線（スコア時系列）＋積み上げ棒（重大度別）・上段サマリカード描画

### Public 化の条件（PoC完了後）
- [ ] 安全ガードレール（ADR-0004 / スコープ allowlist / 危険パス除外）実装済
- [ ] LICENSE / SECURITY.md 整備
- [ ] その後 `gh repo edit ymd38/goodast --visibility public`

---

## コードレビュー backlog（PR #1）

出典: `SuggentionsByCodeReview.md`（Qodo Code Review + PR Agent）

| ID | 指摘 | 種別 | 状態 |
|---|---|---|---|
| Q1 | golangci-lint を `@latest` で未ピン | Reliability | ✅ 修正済 (3c61ee7) |
| Q2 | gitleaks allowlist で docker-compose 全体除外 | Security | ✅ 修正済 (3c61ee7) |
| Q3 | gitleaks を `curl \| tar` で取得（整合性検証なし） | Security | ✅ 修正済（SHA256検証を追加） |
| Q4 | Trivy `exit-code:'0'` でゲート機能なし | Security | ✅ 修正済（`exit-code:'1'`+`continue-on-error`の段階ゲート） |
| Q5 | `http.Server` タイムアウト未設定（api/worker） | Reliability/Security | ✅ 修正済（保守的タイムアウト追加） |
| A1 | `go test -covermode` に `-coverprofile` 無し | - | ✅ 誤検知（検証済・動作OK） |

> **全レビュー指摘に対応済み**（Q1〜Q5 / A1）。Q4 は当面 `continue-on-error: true` で
> PR をブロックしない「段階ゲート」。本ゲート化する際は `continue-on-error` を外す。

### PR #2（DBスキーマ）レビュー backlog

| ID | 指摘 | 対応 |
|---|---|---|
| R1 | scan の不正状態遷移が可能 | ✅ クエリに状態ガード（`AND status=...`）追加。不正遷移は0行→`ErrNoRows` |
| R2 | sqlc が down マイグレーションを読み得る | ✅ schema を `../migrations/*.up.sql` に限定 |
| R3 | `auth_mode='session'` で `enc_headers` NULL 可 | ✅ CHECK制約で session→NOT NULL / none→NULL を強制 |

> throwaway PG で CHECK制約・状態遷移ガードの動作を検証済み。

### PR #3（ADR-0005 river）レビュー backlog

| ID | 指摘 | 対応 |
|---|---|---|
| L1 | `defer tx.Rollback` の errcheck | ✅ `defer func(){ _ = tx.Rollback(ctx) }()` |
| S1 | EnqueueScan に所有確認ゲート無し（ADR-0004 違反） | ✅ enqueue 前に `ownership_verified` 検証 + localhost/127.0.0.1/::1/*.local 例外。純粋関数を unit テスト |
| S2 | Work が非冪等でリトライ時に running のまま詰まる | ✅ StartScan の ErrNoRows 時に GetScan で現状態判定（running→続行 / done・failed→スキップ）。CompleteScan も冪等化 |
| S3 | health server エラー経路で river が Stop されない | ✅ 共通 shutdown ブロックを両経路で通す構造に変更 |

> 結合テストで「unverified→拒否・localhost→許可」「running→再開して done」を検証済み。
> **worker 側の所有確認 defense-in-depth は ADR-0002 に持ち越し**（worker が site をロードして実スキャンする時に再チェック）。

### PR #4（ADR-0002 Nuclei engine）レビュー backlog（Qodo）

| ID | 指摘 | 判定 / 対応 |
|---|---|---|
| Q-4 | 実行失敗で scan が running 固定 | ✅ 最終試行（`job.Attempt>=MaxAttempts`）で `FailScan` 確定。それ以外は再試行 |
| Q-5 | リトライで findings 重複 | ✅ 実行前に `DeleteFindingsByScan` で掃除し再実行を冪等化。結合テストで検証 |
| Q-7 | `markFailed` がDB失敗を握り潰す | ✅ `markFailed` を error 返却化、失敗時は error を返し running 放置を防ぐ |
| Q-8 | Scope がポート未考慮 | ✅ allowlist を host:port（scheme既定で補完）一致に厳格化。unit 100% |
| Q-2 | 危険パスが結果フィルタのみ | ⚠️ 一部対応: `intrusive` タグ除外を追加（dosに加え）。per-request パス遮断は SDK 非対応のため**クロール/認証フェーズへ持ち越し**（下記） |
| Q-6 | スコープ強制がリクエスト時でない | ⚠️ 同上。`WithOptions` は opts 全置換で不可。post-filter を defense-in-depth として維持 |
| Q-1 | ParseSeverity が severity 改変 | ❌ 反論: DB CHECK が `Critical/High/..` 固定。値は1:1保持・大小文字正規化のみ、`unknown`→`Info` は schema 都合の安全コアース |
| Q-3 | CLI parity / 認証スキャン比較テストなし | ❌ 反論: §10 はプロジェクト DoD。認証スキャンは ADR-0003 未実装で本PR範囲外。検知精度検証は専用タスク |

> **持ち越し（重要）**: リクエスト時の host/path 厳密遮断は、単一ターゲット・非クロール（katana無効）・
> DAST fuzzing 無効の現状では逸脱の主経路が限定的。クロール/認証スキャン導入時にカスタム
> transport / redirect ポリシーで実装する（その時点で初めて必要十分になる）。

---

### PR #5（ADR-0004 site feature）レビュー backlog（Qodo / PR Agent）

| ID | 指摘 | 対応 |
|---|---|---|
| V1 | file 方式がリダイレクト追従で所有確認バイパス（Security） | ✅ `DefaultVerifier` の http.Client に `CheckRedirect=noRedirect`（3xx→非200で失敗）。unit テスト |
| V2 | `buildGuide` が nil method で panic（不整合データ） | ✅ `toSiteResponse` を method/token 両 non-nil 時のみ guide 生成に修正 + `buildGuide` を明示引数化 + **migration 000004 で `(verify_method IS NULL)=(verify_token IS NULL)` CHECK 制約**。unit テストで nil 安全性検証 |
| V3 | Register が内部エラーも 400 に誤分類（Correctness） | ✅ `ErrInvalidBaseURL` 追加。409/400/500 に分岐し 500 はログ出力。integration で ftp→400 検証 |
| （PR Agent）repository で部分データ拒否 | ⏭️ V2 の CHECK 制約が根本対策のため repository 追加検証は見送り（service は常に両方セット） |

> gitleaks 誤検知（テスト固定 hex トークン）は `.gitleaks.toml` に exact 値 allowlist で対応済み。

### PR #6（scan 開始エンドポイント）レビュー backlog（Qodo / PR Agent）

| ID | 指摘 | 対応 |
|---|---|---|
| B1 | `ShouldBindJSON` がボディサイズ上限なし（巨大 JSON でリソース枯渇） | ✅ `handler.BodyLimit`（`http.MaxBytesReader`）を router 全ルートに適用（1MiB）。既存 `/sites` 系も同時に保護。unit テストで上限内/超過の両分岐を検証 |
| （PR Agent）Ticket 0005 部分準拠 | ⏭️ 誤検知。存在しない Issue 番号をブランチ名から拾い、マージ済み PR #5 の要件と比較していた |

### PR #7（nuclei CLI parity）レビュー backlog（Qodo）

| ID | 指摘 | 対応 |
|---|---|---|
| P3 | ベースライン CLI のタイムアウト/失敗を握り潰し空 baseline で素通り（Bug） | ✅ `ctx.Err()!=nil` は fatal 化・失敗時に args ログ。加えて **in-scope 0 件を fatal**（正解が無い状態での vacuous pass を防止） |
| P4 | `NUCLEI_TEST_TAGS` の空白未トリムで不正タグ混入（Bug） | ✅ `splitTags` で trim + 空要素除去、空なら fatal |
| P2 | severity/件数を assert せずレポートのみ（Rule 13） | ✅ 共有 template-id の **severity-per-template を hard assert**（同一テンプレ由来で決定的）。生件数は URL 多重度で非決定的なため report 維持（理由を明記） |
| P1 | 認証スキャン未実施（Rule 13 / §10-3） | ✅ 消化。ADR-0003 完了後、`auth_integration_test.go` に認証スキャン検証を追加（`TestNucleiHeaderInjection` 決定的注入証明 + `TestNucleiAuthenticatedCoverage` カバレッジ縮小なし + `make nuclei-auth`）。件数増は非決定的なため B⊇A をハード assert・差分はレポート |

### PR #9（api credential）レビュー backlog（Qodo / PR Agent）

| ID | 指摘 | 対応 |
|---|---|---|
| Bug1 | GetStatus が行の存在だけで configured=true（auth_mode 未評価） | ✅ `auth_mode='session'` 限定で configured。none 行混入でも整合。結合テストに none 行ケース追加 |
| Bug2 | GET が `SELECT *` で不要な enc_headers(bytea) 取得 | ✅ api は復号しないため `SELECT auth_mode, created_at` に限定。upsert も `:exec` 化 |
| Sec | Makefile に固定 dev 鍵をコミット | ✅ 固定鍵廃止 → `make dev-key` で開発者ローカル生成（gitignore）。履歴からも除去（squash） |

### PR #10（worker 復号・注入）レビュー backlog（Qodo / PR Agent）

| ID | 指摘 | 対応 |
|---|---|---|
| W1 | loadHeaders の一過性 DB エラーも即 failed（再試行抑止・Bug） | ✅ `permanentCredentialError`（復号/検証失敗）で分類。恒久→failed / 一過性→river 再試行（最終試行のみ failed）。結合テストで decrypt 失敗→failed を検証 |
| W2 | credential 失敗が掃除前で stale findings 残留（PR Agent） | ✅ `DeleteFindingsByScan` を loadHeaders より前へ移動。decrypt 失敗でも古い findings が残らない（テストで検証） |
| W3 | WithHeaders 全リクエスト注入 → クロスホスト redirect で認証ヘッダ漏えい（Security） | ⏭️ **要ハードニング backlog**（下記）。SDK に redirect 制御 option 関数が無く小修正不可。コメント強化＋緩和策（単一ターゲット・非クロール・intrusive 除外）を明記 |

## 直近のアクション（resume ポイント）

- **未検証で残っている実走確認**:
  1. **§10-3 認証後スキャン**: `make juiceshop-up` → `make nuclei-auth`（`NUCLEI_TEST_TARGET`/`NUCLEI_TEST_TAGS` 上書き可）でローカル PASS 確認。nuclei-templates 導入環境が必要
  2. **API→worker 一気通貫（実 SDK スキャン）**: 自動テストは区間分割（engine 実体 / river・DB・注入到達を fakeEngine / API enqueue）。全経路は api+worker 起動＋curl で手動確認（web UI 実装後に画面から検証予定）
- **次タスク候補（backend セッション）**:
  1. **W3 ハードニング**: クロスホスト redirect の認証ヘッダ漏えい（下記 backlog / ADR-0002 持ち越し）。custom transport + redirect ポリシー
  2. **findings 詳細レポート API**: `GET /scans/:id/findings` 等（レポート画面の入力）
  3. **web (Nuxt) スキャフォールド**（別セッション）→ ダッシュボード描画（Chart.js・`GET /sites/:id/dashboard` を消費）

### ADR-0002 の持ち越し / 留意点
- **【要ハードニング / 認証スキャンで顕在化】クロスホスト redirect での認証ヘッダ漏えい（PR #10 W3）**: `WithHeaders` は SDK 全リクエストにヘッダを付与するが、SDK はリクエスト時の host/path allowlist 強制手段（redirect 制御 option 関数）を持たない。テンプレートが redirects を有効化しクロスホスト redirect が起きると Cookie/Bearer が意図しないホストへ送られ得る。**恒久対策 = クロール/認証スキャン用の custom transport + redirect ポリシー**（既存の「リクエスト時 host 遮断」持ち越しと同一。認証注入導入で優先度上昇）。現状の緩和: 単一ターゲット・非クロール（katana 無効）・破壊的/intrusive タグ除外で逸脱経路を限定。
- **nuclei-templates の取得は未実装**（SDK は既定 catalog 依存）。`make setup` / worker 起動時に固定バージョンを取得する配線は別途（企画書 §12「テンプレート配布」）。`engine/nuclei` の integration テストはテンプレート導入済みを前提にスキップ可能化済み
- **【決定 2026-07-02】nuclei バイナリ/CLI はどのコンテナにも同梱しない**: nuclei は SDK として worker の Go バイナリに静的リンク済みで、実行時に別途 nuclei バイナリは不要。CLI は parity 検証（`make nuclei-parity`）のベースライン比較でのみ `go run @go.mod版`（SDK とバージョン一致）として使い、goodast ランタイムには不要。api への同梱案は ADR-0002 と衝突するため不採用。
  - ただし **nuclei-templates（データ）** は別件。SDK が scan 時に参照するため、固定版取得・worker への同梱は未実装のまま（§12「テンプレート配布」）。api/worker Dockerfile も未作成（docker-compose は `build:` 参照のみ）
- engine のレート/severity/除外タグは現状 `nuclei.DefaultConfig()` のコード定数。運用調整値（レート等）の env 化は必要になった時点で `config` に追加

## メモ（運用）

- マイグレーション適用: `migrate -path migrations -database "$DATABASE_URL" up`
- sqlc 再生成: 各モジュールで `sqlc generate`（v1.31.1）。マイグレーション変更後は必須
- river マイグレーションは CLI で生成: `go run github.com/riverqueue/river/cmd/river@v0.39.0 migrate-get --version N --up`
  - `ALTER TYPE ... ADD VALUE` の制約により、enum 値追加(v4)と使用(v6)は別ファイル(別tx)に分割済み
- 結合テスト実行: DB へ migrate 後 `TEST_DATABASE_URL=... go test -tags=integration ./...`
- ローカル lint は CI と同じ `golangci-lint v2.12.2` を使う。go 1.26.4 ターゲットを lint するには
  リンタも 1.26.4 でビルドする必要がある: `GOTOOLCHAIN=go1.26.4 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
- Makefile 整備済み（`make help`）。Juice Shop は `make juiceshop-up`（compose profile・loopback :3001）、
  実スキャン確認は `make nuclei-scan`（`NUCLEI_TEST_TARGET` / `NUCLEI_TEST_TAGS`）。テンプレ取得は `make nuclei-templates`。
  認証後スキャン検証（§10-3）は `make nuclei-auth`（ヘッダ注入の到達 + 認証カバレッジ縮小なし）

---

## 参照

- 要件・フェーズ計画: `docs/poc-plan.md`
- 意思決定記録（ADR）: `docs/adr/`
- 意思決定ログ（軽量）: `MEMORY.md`
- レビュー原文: `SuggentionsByCodeReview.md`
- バックエンド規約: `.claude/rules/backend.md`
