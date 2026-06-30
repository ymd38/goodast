# PROGRESS — 進行管理

> セッションを跨いで作業を継続するための「現在地」メモ。
> **新しいセッションはまずこのファイルを読み、現在地・次アクションを把握する。**
> **各作業の区切りでこのファイルを更新する。** 決定の経緯は `MEMORY.md`、要件/フェーズは `docs/poc-plan.md` を正とする。

最終更新: 2026-06-29

---

## 現在地スナップショット

- フェーズ: **PoC Phase 1**
- 作業ブランチ: `feat/0005-river-job-queue`
- PR #1（ADR-0001 + CI）/ PR #2（DBスキーマ）: **マージ済み**
- 進行中: ADR-0005 river ジョブキュー（api enqueue ↔ worker dequeue）
- sqlc: **v1.31.1** / river: **v0.39.0**
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
- [ ] ADR-0002 Nuclei SDK 統合（`worker/internal/engine/` の Work() に差込）
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

---

## 直近のアクション（resume ポイント）

1. `feat/0005-river-job-queue` の PR 作成 → CI / PR Agent 確認 → マージ
2. マージ後、**ADR-0002 Nuclei SDK 統合**（`worker/internal/engine/` を実装し ScanWorker.Work() のスタブと差し替え）
3. 並行して **site feature**（サイト登録 + ドメイン所有確認 ADR-0004）、**scan 開始エンドポイント**

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
