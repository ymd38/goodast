# PROGRESS — 進行管理

> セッションを跨いで作業を継続するための「現在地」メモ。
> **新しいセッションはまずこのファイルを読み、現在地・次アクションを把握する。**
> **各作業の区切りでこのファイルを更新する。** 決定の経緯は `MEMORY.md`、要件/フェーズは `docs/poc-plan.md` を正とする。

最終更新: 2026-07-11

---

## 現在地スナップショット

- フェーズ: **PoC Phase 1**
- 作業ブランチ: `main`（**#35 まで マージ済み**・Public 化準備を完了・UI一気通貫フロー完成・手動 E2E 検証済み）
- **Public 化準備一式: 完了・マージ済み**（2026-07-10〜11・PR #33/#34/#35）:
  - **#33 LICENSE / SECURITY.md / NOTICE**: MIT ライセンス採用。SECURITY.md に責任ある利用（自己所有・許可済み対象限定）とガードレール一覧・脆弱性報告窓口（GitHub Private Vulnerability Reporting）を明記。NOTICE に推移的依存のコピーレフト（GPL-2.0 `projectdiscovery/ldapserver`＝Nuclei の LDAP 依存 / MPL-2.0 river・HashiCorp 系ほか）を開示。全履歴 gitleaks（139 commit）クリーン
  - **#34 スキャンジョブ再試行の有界化**: river `MaxAttempts` を 25→3（定数 `scanJobMaxAttempts`）。プリセットのタイムアウト超過（`context.DeadlineExceeded`）は恒久エラーとして即 failed 確定。`markFailed` を `context.WithoutCancel`＋10秒で確定（ctx 失効時に FailScan が道連れ失敗し running 残留する潜在バグを修正）
  - **#35 サイト登録の origin 一意化＋自己スキャン禁止**: origin 正規化を `target.CanonicalOrigin`/`Classify` に集約（既定ポート補完・ループバック別名畳み込み・IPv6 対応・unit 100%）。GOODAST 自身の origin は登録不可（`GOODAST_SELF_ORIGINS` 既定 `localhost:3000,localhost:8080`→400）。migration 000008 で `sites.origin` を UNIQUE 化（既存重複を canonical origin で最古行へ集約・スキャン付け替え・アクティブスキャンは failed 化して `scans_one_active_per_site` 衝突回避）。重複登録は 409＋`existing_site_id`（履歴一元化）
- **残る Public 化ブロッカー: なし（コード面）。** 公開直前の手動作業のみ → 下記「Public 化の条件」参照
- **UI一気通貫フロー v1（コア4画面）: 完成・マージ済み**（2026-07-09・PR #26/#27/#28 → 統合 PR **#31** で main）— サイト登録＋所有確認ガイド（`/sites/new`）→ スキャン設定ウィザード（`/sites/[id]/scan`・プリセット light/standard/deep）→ 進捗ポーリング→結果レポート（`/scans/[id]`・スコア＋重大度色 findings＋専門家導線）。**Juice Shop 相手に実ブラウザで登録→ウィザード→進捗→結果まで完走を確認**。web ゲート 100% 維持。設計 docs は #25、教訓は `MEMORY.md` 2026-07-07/08 エントリ
- **手動 E2E（2026-07-08〜09）で判明した不具合を一式修正・マージ済み**（#29 + #31）:
  - **ローカル対象を登録時点で verified**（`site.Register` が INSERT 1回で `ownership_verified` を確定・部分登録状態も解消）。未確認のままスキャンをブロックしていた件を解決
  - **サイト詳細↔ウィザードのルート衝突**（`[id].vue`＋`[id]/scan.vue` の親子シャドウ）→ `[id]/index.vue` へ改名して兄弟ルート化（「新規スキャンを押しても画面が変わらない」の原因）
  - **登録ページのレイアウト崩れ**（命名スペーシング `--spacing-xl` と `max-w-xl` の名前衝突で 40px 化）→ `max-w-3xl`
  - **同一サイトの同時スキャン拒否**（migration **000007** の部分ユニークインデックス `(site_id) WHERE status IN ('queued','running')`・EnqueueScan の一意制約違反→`ErrScanInProgress`→**409**）
  - **開始ボタンの多重送信ガード＋409表示**（frontend）
  - **スキャンのレート/タイムアウト調整**: ローカル/自己所有対象のみ高レート（100req/s・`ScanProfile.ForLocalTarget`）、外部は保守的 10req/s 維持。タイムアウトを light 15/standard 30/deep 60 分へ延長（light の ~3500テンプレが 5分×10req/s で `context deadline exceeded` になっていた件）
  - **`make db-clean`**: 開発DBのアプリデータ（sites/scans/findings/認証情報/river_job）一掃
- **クロール・ガードレール方針 spec（Phase 2）: 追加・マージ済み**（#30・`docs/superpowers/specs/2026-07-08-crawl-guardrail-policy-design.md`）— クロール段(GET-only+フォーム抽出)/スキャン段(POST含む診断)分離、スコープ強制を post-filter→リクエスト時遡断(ScopedTransport)へ格上げ、動的タイムアウト+絶対上限、二段構え+エンジン非依存。**実装は Phase 2 着手時に別途プラン化**
- **nuclei-templates 固定版取得の配線: 完了・マージ済み**（PR #24）— 固定 tag（既定 v10.4.5）取得を `make`／`setup` に配線、worker 起動時に `templates.Verify` で版突合し fail-fast、SDK は `NUCLEI_TEMPLATES_DIR` を native 解決
- **①正式対応 + ②ドリフト対策: 完了・マージ済み**（PR #23 → main）— スキャンプリセット（軽量/標準/詳細）導入でタグ・タイムアウトのハードコードを撤廃、`summary_json` を共有型 `jobs.ScanSummary` で構造的に固定。migration 000006（`scans.preset`）追加
- **PR #1〜#21 マージ済み**（#19 web スキャフォールド / #20 ローカル開発 DB 修正 / #21 ダッシュボード UI ＋ 手動 E2E バグ修正）
- **ADR-0003 認証情報のアプリ層暗号化: 完結**（3段 PR #8〜#10）— #8 共有 `secrets/`（AES-256-GCM・AAD=siteID）/ #9 api 受付・暗号化保存（`PUT/DELETE/GET /sites/:id/credentials`）/ #10 worker 復号・`engine.ScanRequest.Headers`→nuclei `WithHeaders` 注入。これで**認証後スキャン**が api→worker で通る（詳細は下記ロードマップ）
- **§10-3 認証後スキャン検証: 完了**（#11）— `worker/internal/engine/nuclei/auth_integration_test.go`（`TestNucleiHeaderInjection` 決定的注入証明 / `TestNucleiAuthenticatedCoverage` カバレッジ縮小なし）+ `make nuclei-auth`。**残: ローカルで `make juiceshop-up` → `make nuclei-auth` 実走 PASS 確認**（nuclei-templates 導入環境が必要）
- **スコア計算: 完了**（#12）— `api/internal/report/score.go`（`Compute`/`Score`/`Band`/`Delta`・§5.1 の式・[0,100] クランプ・色は Band で frontend にマップ・unit 100%）
- **ダッシュボード集計 backend: 完了**（#13）— `api/internal/report/`（`dashboard.go` 純粋集計 / `repository.go` / `service.go`）+ `handler/dashboard.go`（`GET /sites/:id/dashboard`：最新スコア＋前回差分＋スコア時系列）+ sqlc `ListDoneScanSummaries`。**残: frontend（Chart.js 描画・別セッション）**
- **スキャン結果 API（状態/明細分離）: 完了**（#15）— `handler/scan_result.go`（`report.Service` 依存）: `GET /scans/:id`（状態＝status＋summary＋score・診断中は 200＋status で進捗提示・summary は done で非 nil）/ `GET /scans/:id/findings`（明細＝重大度順）。404 は scan 不在時のみ。sqlc `ListFindingsByScan`・`ScanExists`。**残: frontend（結果レポート画面・別セッション）**
- **W3 ハードニング: 対応済み**（#16）— 認証注入時のみ `DisableRedirects` でクロスホスト redirect の認証ヘッダ漏えいを遮断（統合テストで実 SDK 実証）
- **診断履歴 API: 完了**（作業ブランチ）— `GET /sites/:id/scans`（全 status を新しい順・各エントリは `GET /scans/:id` と同形）。既存 `ListScansBySite` 再利用＋`toScanState` 抽出（DRY）。DashboardHandler 相乗り。未知サイト=200＋空。**残: frontend（診断履歴画面・別セッション）**
- **backend の PoC 主要 API は出揃った**。次: **web (Nuxt) スキャフォールド → UI 描画**（別セッション推奨）。残 backend 小粒は findings ステータス更新（false_positive 化・PoC 表示系には不要・見送り中）
- **web (Nuxt) スキャフォールド: 完了**（PR A）— Nuxt 3 + Tailwind v4（tokens.css @theme 化）+ Vitest（カバレッジ100%ゲート）+ ESLint + openapi-typescript 型付きクライアント（swagger 2.0→OpenAPI 3 変換経由・生成物コミット）+ CI frontend/pnpm-audit 有効化。**残: ダッシュボード画面（PR B・同設計スペック）**
- **ダッシュボード frontend: 完了・マージ済み**（PR #21）— サイト一覧（`GET /sites`）→ サイト別ダッシュボード（`/sites/[id]`・§6-6 の3層: ScoreCard＋SeverityCountCards / ScoreTrendChart / SeverityStackChart）。chart config は純粋関数（unit 100%）・canvas は薄ラッパ＋モック・色は tokens.css の CSS 変数を実行時解決で注入。history<2 は「データ不足」。**残: サイト登録・スキャン設定ウィザード・結果レポート・診断履歴 の各画面**
- **ローカル開発 DB 修正: 完了**（PR #20）— `DATABASE_URL` を 127.0.0.1 に統一（IPv6 ::1 解決ミスマッチ回避）／ Dockerfile 未作成の api・worker・web を `profiles: [app]` で隔離し素の `docker compose up` は db のみ起動
- **手動 E2E で 2 バグを対応**（PR #21 同梱）— ②summary_json デコード不一致（カウント 0 化）は**本修正済み**、①UI 起点スキャンが完走しない件は**暫定回避のみ・backend 正式対応が残**（下記「手動 E2E で判明した課題」を参照）
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
- [x] ADR-0003 認証情報のアプリ層暗号化（`scan_credentials.enc_headers`）※PR #8〜#10 で完結（上記スナップショット参照）
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
  - `SeverityCounts` の json タグは worker の counts フィールドと一致。ただし worker は counts を `{"findings": {...}}` と **ネストして** summary_json に書き込むため、api 側は `decodeSummaryCounts`（`findings` キー経由）でデコードする。※当初この点を誤り（フラット直デコード）カウント 0 化のバグ（②）を招いた。PR #21 で修正済み（下記「手動 E2E で判明した課題」§）
  - テーブル駆動テストで境界値・クランプ（負数カウント含む）・全バンド・Delta・NewScore 範囲外を網羅。**unit 100%**・lint 0 issues
- [x] web (Nuxt) スキャフォールド → CI の frontend / pnpm-audit ジョブ有効化
- [x] ダッシュボード（スコア + 時系列・Chart.js）
  - **backend 集計 API 完了**（`api/internal/report/` + `handler/dashboard.go`）: `GET /sites/:id/dashboard`
    - sqlc `ListDoneScanSummaries`（done かつ summary_json あり を日付昇順）を追加
    - `dashboard.go`（純粋集計 `BuildDashboard`・unit 100%）: 最新スコア＋前回差分（初回は null）＋スコア時系列（history 昇順）。`Score`/`Band` を消費
    - `repository.go`（summary_json→SeverityCounts デコード境界・Date は finished_at 優先）/ `service.go`（gin 非依存）/ `handler/dashboard.go`（uuid 400 / スキャン無し=200＋latest:null・history:[]）
    - 結合テスト（throwaway PG）で 400・空・集計＋除外（queued / summary_json NULL）を検証。実 DB 実走 PASS
- [x] スキャン結果 API（§6.3 進捗 / §6.4 結果レポート）※backend + frontend 完了（結果レポート画面は #31 で main）
  - **backend 完了**（`api/internal/report/scan_result.go` + `handler/scan_result.go`）: 状態と明細を分離
    - `GET /scans/:id`（状態）: status＋summary（counts＋score/band/label）。診断はバックグラウンド実行のため**scan が存在する限り 200＋status で進捗提示**（queued/running）、summary は done で非 nil。進捗ポーリング兼用
    - `GET /scans/:id/findings`（明細）: findings を重大度順（Critical→Info・SQL CASE）。クリーン（0件）は 200＋空配列
    - **404 は scan 行が無いときのみ**（不在／未完了／クリーンは 404 と status で区別）。不正 uuid=400
    - sqlc `ListFindingsByScan`・`ScanExists`（状態は既存 `GetScan` 再利用）。純粋ヘルパ `buildScanSummary` unit 100%。結合テスト（throwaway PG・8ケース）実 DB 実走 PASS
  - **残（frontend・別セッション）**: 結果レポート画面（重大度カラー・CWE・検出URL・修正方法・専門家相談導線）、進捗表示

### Public 化の条件（PoC完了後）
- [x] 安全ガードレール（ADR-0004 / スコープ allowlist / 危険パス除外）実装済
- [x] LICENSE / SECURITY.md / NOTICE 整備（#33）
- [x] 自己スキャン禁止・origin 一意化（#35）／スキャン再試行の有界化（#34）
- [ ] **公開直前の手動作業**: GitHub 設定で Private Vulnerability Reporting を有効化（Settings → Code security）。SECURITY.md がこの窓口を案内しているため
- [ ] その後 `gh repo edit ymd38/goodast --visibility public`（**不可逆操作**・ユーザー明示指示で実行）
> **コード面のブロッカーは解消済み。** Dockerfile ×3（api/worker/web）は Public 化の必須要件ではなく、デプロイ時タスクとして分離（下記「次タスク候補」）。

---

## 手動 E2E で判明した課題（2026-07-05・feat/0020-dashboard 上で対応）

UI から Juice Shop を実スキャンして初めて出た問題。UI/配管は正常に動作した。

| # | 問題 | 状態 |
|---|---|---|
| ② | `summary_json` デコード不一致で重大度カウントが常に 0 化（worker はネスト `{"findings":{...}}`、api はフラットで Unmarshal） | ✅ **本修正済み**（69774ce）。`decodeSummaryCounts` に集約 + 結合テストのフィクスチャをネスト形へ修正（回帰固定）+ 純粋関数ユニットテスト追加 |
| ① | UI 起点スキャンが完走しない（`DefaultConfig` の Tags 空＝全 13k テンプレ × river 既定 1 分タイムアウトで context deadline exceeded を 25 回リトライ） | ✅ **本修正済み**（`feat/scan-preset-and-summary-contract`）。スキャンプリセット（軽量/標準/詳細）を導入しタグ・タイムアウトのハードコードを撤廃。②のドリフトも共有型 `jobs.ScanSummary` で構造的に固定（下記参照） |

### ①の正式対応（完了・`feat/scan-preset-and-summary-contract`）
- **スキャンプリセット（軽量/標準/詳細）を実装済み**（企画書 §6-2）。プリセット識別子は共有 `jobs.Preset`（api は engine を import 不可のため・ADR-0002）、タグ/レート/タイムアウトへの写像は `engine.PlanFor`（unit 100%）。値: light=`misconfig,tech,exposure`/5分、standard=+`exposed-panels,default-login,cve`/15分、deep=+`xss,sqli,lfi,ssrf,rce,takeover`/30分、共通 exclude `dos,intrusive`・rate 10/s。deep も全 13k は回さずタグで有界化
- **preset の伝播**: `POST /scans {site_id, preset?}`（省略時 standard・不正値 400）→ `scans.preset` カラム（migration 000006）に永続化 ＋ `jobs.ScanArgs.Preset` にも載せる（river の `Timeout()` callback は context/DB を持てないため）。worker は `engine.PlanFor(preset)` で `ScanRequest.Profile` と Timeout を導出。`nuclei.DefaultConfig()` ハードコードは撤廃、`nuclei.New()` は per-scan Profile 参照
- **ドリフト対策（②の根本）完了**: `summary_json` の形を共有型 `jobs.ScanSummary{Findings jobs.SeverityCounts}` に一元化。worker（`engine.Summarize` 返却）と api（`report.decodeSummaryCounts`）の双方がこの型を経由し、`report.SeverityCounts(jobs.SeverityCounts)` の型変換で橋渡し。形が乖離すればコンパイルエラーになり、静かなドリフトが構造的に起きない

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
| W3 | WithHeaders 全リクエスト注入 → クロスホスト redirect で認証ヘッダ漏えい（Security） | ✅ 対応済み（作業ブランチ）。**認証ヘッダ注入時に限り `DisableRedirects`**（`WithOptions(types.DefaultOptions()+DisableRedirects)` を先頭適用）で redirect 追従を停止。SDK の httpclientpool がテンプレの `redirects:true` すら上書きし全 redirect を DontFollowRedirect に強制することを確認。統合テスト（2 httptest サーバ＋redirect 追従テンプレ）で「fix 無し=漏れる / fix あり=漏れない」を実 SDK 実証。未認証は挙動不変（§10 parity 無影響） |

## 直近のアクション（resume ポイント）

> UI一気通貫（登録→ウィザード→進捗→結果）は #31 まで、Public 化準備（#33/#34/#35）まで main マージ済み。
> オープン中の PR は無し。**コード面の Public 化ブロッカーは解消済み**（残るは公開直前の手動作業のみ・上記「Public 化の条件」）。以下は未着手・未検証の残タスク。

- **次タスク候補（backend セッション）**:
  1. ~~**river `max_attempts` の見直し**~~ → **#34 で対応済み**（`scanJobMaxAttempts=3`＋`context.DeadlineExceeded` は即 failed）
  2. **findings ステータス更新**（false_positive / fixed マーク）等の残 API
  3. **Dockerfile ×3**（api / worker / web）— 未作成。compose の api/worker/web は `profiles: [app]` で隔離中。揃えば `docker compose --profile app up` で有効化（デプロイに必要・Public 化の必須要件ではない）
- **次タスク候補（frontend セッション）**:
  1. **診断履歴画面**（`GET /sites/:id/scans`・backend 完了済み）— UI 一気通貫の4画面には含めず未着手。サイト詳細から履歴一覧への導線
  2. **認証情報入力 UI**（`PUT/DELETE/GET /sites/:id/credentials`・backend 完了済み・ADR-0003）— UI 未実装。認証後スキャンを UI から使うために必要（Cookie/Bearer をマスク表示で入力）
  3. **a11y バッチ**（ErrorNotice atom 抽出＋`role="alert"`/`aria-live`・プリセットカード `aria-pressed`）— UI 最終レビューの follow-up。エラーバンド `border-m-red` の `<p>` が複数箇所に重複しているのを atom 化する契機に
- **未検証で残っている実走確認**:
  1. **§10-3 認証後スキャン**: `make juiceshop-up` → `make nuclei-auth`（`NUCLEI_TEST_TARGET`/`NUCLEI_TEST_TAGS` 上書き可）でローカル PASS 確認
- **Public 化の条件**（上記ロードマップ §Public 化）: コード面は完了（#33/#34/#35）。残りは公開直前の手動作業（GitHub Private Vulnerability Reporting 有効化）→ `gh repo edit ymd38/goodast --visibility public`（不可逆・明示指示で実行）

### ADR-0002 の持ち越し / 留意点
- **【対応済み】クロスホスト redirect での認証ヘッダ漏えい（PR #10 W3）**: 認証ヘッダ注入時に限り `DisableRedirects=true`（`WithOptions(types.DefaultOptions()+DisableRedirects)` を先頭適用）で redirect 追従を停止し、Cookie/Bearer/任意の認証ヘッダが別ホストへ送出される経路を塞いだ。SDK の httpclientpool がテンプレの `redirects:true` を上書きし全 redirect を DontFollowRedirect に強制することを確認済み。統合テストで実 SDK 実証。**留意**: これは redirect 全停止の保守的対策。将来クロール/認証スキャンで「同一ホスト redirect は追従」等の細かな制御が要る場合は custom transport + redirect ポリシーを検討（現 PoC 範囲では DisableRedirects で十分）。なお **リクエスト時の host/path 厳密遮断（スコープ強制）** は別課題として引き続き post-filter（`scope.Allows`）で担保（クロール導入時に custom transport で強化）。
- **【対応済み】nuclei-templates 固定版取得の配線**（ブランチ `feat/nuclei-templates-pinning`・企画書 §12）: `make nuclei-templates` を `git clone --depth 1 --branch $(NUCLEI_TEMPLATES_VERSION)`（既定 `v10.4.5`）方式に変え固定 tag を `NUCLEI_TEMPLATES_DIR`（既定 repo 直下 `nuclei-templates/`・gitignore）へ取得、`.goodast-templates-version` マーカーを書く（冪等: 版一致ならスキップ）。`make setup` から呼ぶ。worker は起動時 `templates.Verify`（unit 100%）でマーカーと固定版を突合し、不在/不一致なら **fail-fast**（cipher/DB より前）。SDK は同じ `NUCLEI_TEMPLATES_DIR` を native に解決するためコード変更不要（os.Setenv しない）。実クローン（13320 テンプレ）・冪等スキップ・版不一致 fail-fast をローカル実走確認済み
- **【決定 2026-07-02】nuclei バイナリ/CLI はどのコンテナにも同梱しない**: nuclei は SDK として worker の Go バイナリに静的リンク済みで、実行時に別途 nuclei バイナリは不要。CLI は parity 検証（`make nuclei-parity`）のベースライン比較でのみ `go run @go.mod版`（SDK とバージョン一致）として使い、goodast ランタイムには不要。api への同梱案は ADR-0002 と衝突するため不採用。
  - **nuclei-templates（データ）の固定版取得は #24 で実装済み**（`make nuclei-templates`・worker 起動時 `templates.Verify` で fail-fast）。ただし **worker イメージへの同梱（Dockerfile）は未作成**（docker-compose は `build:` 参照のみ・上記「Dockerfile ×3」タスク参照）
- engine のレート/severity/除外タグ/タイムアウトは `engine.PlanFor(preset)` の定数（`nuclei.DefaultConfig()` は撤廃済み）。ローカル対象は `ScanProfile.ForLocalTarget` でレートを引き上げ。運用調整値（レート等）の env 化は必要になった時点で `config` に追加

## メモ（運用）

- マイグレーション適用: `migrate -path migrations -database "$DATABASE_URL" up`（ホスト実行・DB は 127.0.0.1:5432 の loopback 公開）。
  接続先は `localhost` でなく **127.0.0.1**（IPv6 ::1 解決とのミスマッチ回避）。DB が「Up なのに接続拒否」の場合は
  Docker Desktop のポート転送が外れている（`docker compose ps` の PORTS に `->` が無い）→ `docker compose up -d --force-recreate db`
- compose の api / worker / web は **Dockerfile 未作成のため `profiles: [app]` で隔離**。素の `docker compose up` は db のみ起動。
  コンテナ化（Dockerfile 3本）は別タスク。揃ったら `docker compose --profile app up` で有効化
- sqlc 再生成: 各モジュールで `sqlc generate`（v1.31.1）。マイグレーション変更後は必須
- river マイグレーションは CLI で生成: `go run github.com/riverqueue/river/cmd/river@v0.39.0 migrate-get --version N --up`
  - `ALTER TYPE ... ADD VALUE` の制約により、enum 値追加(v4)と使用(v6)は別ファイル(別tx)に分割済み
- 結合テスト実行: DB へ migrate 後 `TEST_DATABASE_URL=... go test -tags=integration ./...`
- ローカル lint は CI と同じ `golangci-lint v2.12.2` を使う。go 1.26.4 ターゲットを lint するには
  リンタも 1.26.4 でビルドする必要がある: `GOTOOLCHAIN=go1.26.4 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
- Makefile 整備済み（`make help`）。Juice Shop は `make juiceshop-up`（compose profile・loopback :3001）、
  実スキャン確認は `make nuclei-scan`（`NUCLEI_TEST_TARGET` / `NUCLEI_TEST_TAGS`）。テンプレ取得は `make nuclei-templates`。
  認証後スキャン検証（§10-3）は `make nuclei-auth`（ヘッダ注入の到達 + 認証カバレッジ縮小なし）
- **OpenAPI(Swagger)**: handler の swaggo 注釈から `make swagger` で `api/internal/docs`（swagger.json/yaml）を生成。
  API サーバ起動中は `GET /swagger/index.html` で UI 配信。**API 変更時は pre-commit hook が自動再生成**（`make setup` で `core.hooksPath=.githooks` 有効化）。
  **frontend は `api/internal/docs/swagger.yaml` を正として `openapi-typescript` で型付きクライアントを生成する**（別セッション）。swag/gin-swagger のバージョンは Makefile `SWAG_VERSION` と go.mod で固定。

---

## 参照

- 要件・フェーズ計画: `docs/poc-plan.md`
- 意思決定記録（ADR）: `docs/adr/`
- 意思決定ログ（軽量）: `MEMORY.md`
- レビュー原文: `SuggentionsByCodeReview.md`
- バックエンド規約: `.claude/rules/backend.md`
