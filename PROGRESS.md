# PROGRESS — 進行管理

> セッションを跨いで作業を継続するための「現在地」メモ。
> **新しいセッションはまずこのファイルを読み、現在地・次アクションを把握する。**
> **各作業の区切りでこのファイルを更新する。** 決定の経緯は `MEMORY.md`、要件/フェーズは `docs/poc-plan.md` を正とする。

最終更新: 2026-06-29

---

## 現在地スナップショット

- フェーズ: **PoC Phase 1**
- 作業ブランチ: `feat/0001-api-worker-separation`
- PR: **#1 OPEN** → base `main`（https://github.com/ymd38/goodast/pull/1）
- CI: レビュー対応コミット push 済み → 再実行結果待ち
- レビュー: PR #1 の全指摘（Q1〜Q5 / A1）対応済み
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

### 実装（未着手・順不同の候補）
- [ ] DBスキーマ: `migrations/` + sqlc セットアップ（企画書 §5）
- [ ] ADR-0005 river ジョブキュー（api enqueue ↔ worker dequeue）
- [ ] ADR-0002 Nuclei SDK 統合（`worker/internal/engine/`）
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

---

## 直近のアクション（resume ポイント）

1. **CI #1 の結果確認** — レビュー対応 push 後の再実行を確認（緑なら次へ／赤なら原因対応）
2. PR #1 レビュー再確認 → 問題なければマージ
3. PR #1 マージ後、**DBスキーマ** または **ADR-0005 river** に着手

---

## 参照

- 要件・フェーズ計画: `docs/poc-plan.md`
- 意思決定記録（ADR）: `docs/adr/`
- 意思決定ログ（軽量）: `MEMORY.md`
- レビュー原文: `SuggentionsByCodeReview.md`
- バックエンド規約: `.claude/rules/backend.md`
