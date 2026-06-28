# goodast

## What is this

UI起点・初心者向けのOSS Web脆弱性スキャナー（DAST）。Nuclei v3 Go SDKをラップし、Webアプリの動的検査をブラウザから実行する。PoC段階の責務は「Nucleiラッパー + UI + 非同期ジョブ + 可視化ダッシュボード」のみ（AIはフェーズ2以降）。
> 要件・背景・フェーズ計画は `docs/poc-plan.md` を参照。

## Quick Start

```bash
# 依存セットアップ
make setup              # Go modules + npm install + nuclei-templates 取得

# DB（PostgreSQL）起動 + マイグレーション
make db-up
make migrate

# 開発起動（api / worker / web を個別に）
make dev-api            # Go / Gin API server
make dev-worker         # Go scan worker（Nuclei SDK隔離）
make dev-web            # Nuxt 3

# テスト
make test               # 全テスト
make test-api / test-worker / test-web

# 検証環境（OWASP Juice Shop）
make juiceshop-up       # Docker で Juice Shop 起動
```
> Makefileのターゲットは実装時に確定。上記は想定インターフェース。

## Architecture

3プロセス構成。**APIとスキャンワーカーは必ず分離する**（理由は ADR-0001）。

- **api/**（Go / Gin）: スキャン受付、サイト・履歴管理、ドメイン所有確認、認証情報の暗号化保管、ダッシュボード用データ提供。
- **worker/**（Go）: Nuclei SDK をここに**のみ**置く。`river` でデキューしたジョブを非同期実行し、結果をコールバックで受けてDB保存。
- **web/**（Nuxt 3）: UI起点の全操作。サイト登録、スキャン設定ウィザード、レポート、履歴、ダッシュボード。

**データフロー**: web → api（ジョブをenqueue / river）→ PostgreSQL → worker（dequeue）→ Nuclei実行 → findings保存 → web（ダッシュボード描画）。

## Key Conventions

- **言語**: api/worker は Go、web は Nuxt 3（TypeScript）。既存スタック（右腕ダイレクト）に準拠。
- **デザイン**: web（UI）の実装は `DESIGN.md` のデザインシステムに従う。色・タイポグラフィ・余白・コンポーネント仕様はトークンを参照し、ハードコード（生のhex等）しない。
- **ジョブキュー**: `riverqueue/river`。新たなブローカー（Redis等）を増やさない。
- **暗号化**: 認証情報（Cookie/Bearer）は**アプリケーションレイヤー**で暗号化（DB側pgcryptoは使わない。ADR-0003）。
- **重大度**: PoCではNucleiテンプレートの `severity` をそのまま使用。スコア計算は `internal/report` に集約。
- **DB**: マイグレーションは `migrations/` で管理。スキーマ定義は企画書 §5 を正とする。
- **意思決定ログ**: 議論を経て確定した決定事項・新発見・プランの見直しは `MEMORY.md` に追記する。フォーマルなADRを起こすほどでない粒度の「なぜそうしたか」の記録先として使う。

## Critical Constraints

> ここが最重要。違反すると安全性・設計が崩れる。

- **Nuclei SDK は `worker/` にのみ置く。** api/ から直接importしない。Nucleiバージョンは固定（ADR-0002）。
- **認証情報・APIキーは平文ログ禁止。** APIレスポンスでも生値を返さない（マスク表示）。
- **ドメイン所有確認を通すまでスキャン実行不可。** ただし `localhost` `127.0.0.1` `::1` `*.local` は確認スキップ（ADR-0004）。
- **スコープは登録ドメインのallowlistに限定。** 外部ドメインへの逸脱を禁止。
- **危険パスはデフォルト除外**（`logout` `signout` `delete` `remove` `destroy` `admin/*` 等）。認証後スキャンでログアウト・データ削除を踏まない。
- **保守的なデフォルトレート**（低req/s・低並列）。破壊的テンプレートはデフォルト無効・明示オプトインのみ。

## 作業ルール

### セッション開始時（厳守）
作業を始める前に **`PROGRESS.md` を読み、現在地・直近のアクション・レビュー backlog を把握する。**
作業の区切り（コミット・PR・タスク完了時）には `PROGRESS.md` を最新化する。これによりセッションを跨いでも継続できる。

### Plan First（厳守）
実装を始める前に、必ず以下の順序で進める。承認なしに実装へ進んではならない。

1. **調査**: 関連ファイル・依存関係を正確に調査する
2. **計画**: 変更箇所・実装方針・影響範囲をテキストで提示する
3. **承認**: ユーザーの「進めてください」を受けてから実装開始
4. **実装**: 計画に沿って実装する
5. **検証**: 対象の `make test-api` / `make test-worker` / `make test-web` でパスを確認

> 「とりあえず実装してみる」は禁止。必ず計画を言語化してから手を動かすこと。

### タスクごとの必須参照ファイル

| タスク | 参照ファイル |
|---|---|
| Go バックエンド実装（api/ / worker/） | `.claude/rules/backend.md` |
| UI・コンポーネント実装（web/） | `.claude/rules/frontend.md` + `DESIGN.md` |
| デザイントークン確認 | `web/assets/css/tokens.css`（値の正）、セマンティックルールは `.claude/rules/frontend.md` |
| git 操作・PR作成 | `.claude/rules/git.md` |
| Issue → PR 一気通貫 | `.claude/agents/issue-to-pr.md` |

### その他ルール
- フロント（web/）とバックエンド（api/ / worker/）は**別セッション**で作業する（コンテキスト汚染防止）
- 曖昧な指示・矛盾がある場合は必ず質問すること
- 決定事項・新発見・プランの見直しは `MEMORY.md` に追記する

## Testing

- **ユニット/統合**: `make test`。
- **検知精度の検証**: OWASP Juice Shop（`make juiceshop-up`）を対象に、
  1. Nuclei CLI で直接スキャン → ベースライン（正解）
  2. goodast 経由でスキャン → 件数・severityが一致すること（欠落ゼロ）
  3. 認証後スキャン（Juice ShopのCookie持ち込み）で未認証時よりfindingが増えること
- 詳細フローは企画書 §10。

## References

- 企画書（要件・フェーズ計画）: `docs/poc-plan.md`
- 意思決定記録（ADR）: `docs/adr/`
- DBスキーマ: `migrations/` ＋ 企画書 §5
- デザインシステム（色・タイポグラフィ・コンポーネント）: `DESIGN.md`
- 意思決定ログ（軽量・会話起点）: `MEMORY.md`
- 進行管理（現在地・次アクション・レビューbacklog）: `PROGRESS.md`
