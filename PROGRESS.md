# PROGRESS — 進行管理

> セッションを跨いで作業を継続するための「現在地」メモ。
> **新しいセッションはまずこのファイルを読み、現在地・次アクションを把握する。**
> **各作業の区切りでこのファイルを更新する。** 決定の経緯は `MEMORY.md`、要件/フェーズは `docs/poc-plan.md` を正とする。

最終更新: 2026-06-30

---

## 現在地スナップショット

- フェーズ: **PoC Phase 1**
- 作業ブランチ: `feat/0002-nuclei-engine`
- PR #1（ADR-0001 + CI）/ PR #2（DBスキーマ）/ PR #3（ADR-0005 river）: **マージ済み**
- 進行中: ADR-0002 Nuclei SDK 統合（`worker/internal/engine/` 実装 + `Work()` 差込）
- sqlc: **v1.31.1** / river: **v0.39.0** / **Nuclei SDK: v3.9.0（go.mod 固定）**
- モジュール構成: api / worker / **jobs（共有ジョブ契約・依存ゼロ）** の3モジュール（go.work + replace）
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
- [ ] Makefile（`make dev-api` 等の想定ターゲット）

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
- [ ] スキャン開始 HTTP エンドポイント（scan feature・EnqueueScan を呼ぶ）
- [ ] ADR-0004 ドメイン所有確認（ファイル設置 / DNS TXT）
- [ ] ADR-0003 認証情報のアプリ層暗号化（`scan_credentials.enc_headers`）
- [ ] サイト登録 / スキャン受付 API（site / scan feature）
- [ ] スコア計算（`internal/report`）
- [ ] web (Nuxt) スキャフォールド → CI の frontend / pnpm-audit ジョブ有効化
- [ ] ダッシュボード（スコア + 時系列・Chart.js）

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

---

## 直近のアクション（resume ポイント）

1. `feat/0002-nuclei-engine` の PR 作成 → CI / PR Agent 確認 → マージ
2. **site feature**（サイト登録 + ドメイン所有確認 ADR-0004）、**scan 開始 HTTP エンドポイント**（`EnqueueScan` を呼ぶ）
3. **ADR-0003 認証情報のアプリ層暗号化** → ここで worker に credential ロード＋復号＋ヘッダ注入（`engine.ScanRequest` にヘッダ受け口を追加し session スキャンを通す）
4. Juice Shop で検知精度の検証（Nuclei CLI ベースライン vs goodast）: `NUCLEI_TEST_TARGET=http://localhost:3000` で `engine/nuclei` の integration テスト実行

### ADR-0002 の持ち越し / 留意点
- **nuclei-templates の取得は未実装**（SDK は既定 catalog 依存）。`make setup` / worker 起動時に固定バージョンを取得する配線は別途（企画書 §12「テンプレート配布」）。`engine/nuclei` の integration テストはテンプレート導入済みを前提にスキップ可能化済み
- engine のレート/severity/除外タグは現状 `nuclei.DefaultConfig()` のコード定数。運用調整値（レート等）の env 化は必要になった時点で `config` に追加

## メモ（運用）

- マイグレーション適用: `migrate -path migrations -database "$DATABASE_URL" up`
- sqlc 再生成: 各モジュールで `sqlc generate`（v1.31.1）。マイグレーション変更後は必須
- river マイグレーションは CLI で生成: `go run github.com/riverqueue/river/cmd/river@v0.39.0 migrate-get --version N --up`
  - `ALTER TYPE ... ADD VALUE` の制約により、enum 値追加(v4)と使用(v6)は別ファイル(別tx)に分割済み
- 結合テスト実行: DB へ migrate 後 `TEST_DATABASE_URL=... go test -tags=integration ./...`
- ローカル lint は CI と同じ `golangci-lint v2.12.2` を使う。go 1.26.4 ターゲットを lint するには
  リンタも 1.26.4 でビルドする必要がある: `GOTOOLCHAIN=go1.26.4 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
- Makefile（`make migrate` / `make sqlc` 等）は未整備（TODO）

---

## 参照

- 要件・フェーズ計画: `docs/poc-plan.md`
- 意思決定記録（ADR）: `docs/adr/`
- 意思決定ログ（軽量）: `MEMORY.md`
- レビュー原文: `SuggentionsByCodeReview.md`
- バックエンド規約: `.claude/rules/backend.md`
