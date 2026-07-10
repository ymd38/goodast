# goodast

UI起点・初心者向けのOSS Web脆弱性スキャナー（DAST）。Nuclei v3 Go SDK をラップし、Webアプリの動的検査をブラウザから実行する。

> **⚠️ 開発ステータス: 早期開発段階（PoC Phase 1）**
>
> 本プロジェクトはまだ**開発の初期段階**であり、仕様・API・スキーマは予告なく変更されます。本番利用は想定していません。主なロードマップは以下のとおりです。
>
> - **探索（クロール）機能**: 現在は指定した**単一 URL** に対する診断のみに対応。次フェーズでサイト内リンク・フォームをたどる**探索機能**の実装を予定。
> - **スキャンエンジン**: 現在は**Nuclei のみ**。加えて **OWASP ZAP** を用いたエンジンの取り組みも計画中（`worker/internal/engine/` はエンジン差し替えを見据えた構成）。

詳細は [CLAUDE.md](CLAUDE.md) と [docs/poc-plan.md](docs/poc-plan.md) を参照。

## アーキテクチャ

3プロセス構成（**API とスキャンワーカーは分離**・ADR-0001）。

- **api/**（Go / Gin）— スキャン受付・サイト/履歴管理・所有確認・認証情報の暗号化保管・ダッシュボードデータ
- **worker/**（Go）— Nuclei SDK を**ここにのみ**隔離（ADR-0002）。`river` でデキューし非同期スキャン
- **web/**（Nuxt 3）— UI 起点の全操作（サイト登録＋所有確認ガイド・スキャン設定ウィザード・進捗表示・結果レポート・ダッシュボード）

データフロー: web → api（enqueue / river）→ PostgreSQL → worker（dequeue → Nuclei 実行 → findings 保存）→ web（ダッシュボード描画）

## 安全ガードレール

誤用・事故を防ぐため、以下を既定で強制する（詳細は [SECURITY.md](SECURITY.md) / [docs/adr/](docs/adr/)）。

- **ドメイン所有確認**: 所有確認（ファイル設置 / DNS TXT）を通すまで外部ドメインはスキャン不可（ADR-0004。`localhost` / `127.0.0.1` / `::1` / `*.local` のローカル対象は確認スキップ）
- **自己スキャン防止**: goodast 自身の origin（ドメイン+ポート・`GOODAST_SELF_ORIGINS`）は登録・スキャン不可
- **スコープ限定**: スキャンは登録ドメインの allowlist に限定し、外部ドメインへの逸脱を禁止
- **危険パスの既定除外**: `logout` / `signout` / `delete` / `admin/*` 等、破壊的操作を踏み得るパスを除外
- **保守的な既定レート**: 外部対象は低レート・低並列。破壊的テンプレート（`dos` / `intrusive`）は既定無効・明示オプトインのみ
- **認証情報の暗号化**: 持ち込み Cookie/Bearer はアプリケーションレイヤーで暗号化保存（ADR-0003）。ログ・API レスポンスに平文を出さない（マスク表示）

## Quick Start

タスクは Makefile に集約。`make help` で一覧を表示する。

```bash
make setup          # Go モジュール依存を取得
make db-up          # PostgreSQL を docker-compose で起動
make migrate        # マイグレーション適用

make dev-api        # API サーバ起動
make dev-worker     # スキャンワーカー起動

make test           # 全 Go ユニットテスト（race）
make lint           # golangci-lint（CI と同設定）
```

DB 接続情報は `DATABASE_URL` 変数で上書き可能（既定はローカル開発用 `docker-compose` の値）。

## 検証環境（OWASP Juice Shop）

検知精度の検証には [OWASP Juice Shop](https://owasp.org/www-project-juice-shop/) を使う（現代的な SPA + REST API + 認証を備え、Nuclei 公式テンプレートも対応）。意図的に脆弱なため、`docker-compose` で **loopback 限定**（`http://localhost:3001`）で起動する。

```bash
make juiceshop-up        # Juice Shop を起動（http://localhost:3001）
make nuclei-templates    # nuclei-templates を固定版で取得（初回のみ）
make nuclei-scan         # worker の engine を実対象に対して結合テスト

make juiceshop-down      # 停止・削除
```

`make nuclei-scan` は `NUCLEI_TEST_TARGET`（既定 `http://localhost:3001`）に対し worker のスキャンエンジン（`worker/internal/engine/nuclei`）を実行する結合テスト。テンプレートはタグで絞り込め、`NUCLEI_TEST_TAGS`（既定 `misconfig,tech`）で変更できる。

> 検知精度の検証フロー（Nuclei CLI ベースラインとの件数一致・認証後スキャンのカバレッジ拡大）は [docs/poc-plan.md §10](docs/poc-plan.md) を参照。

## テスト

- ユニット: `make test`（`-race`）
- 結合（要 DB）: `make test-integration`（`TEST_DATABASE_URL` を指定し migrate 済みの DB が必要）
- カバレッジ: `make cover`

Nuclei SDK を呼ぶテストは `//go:build integration` で分離している（ネットワーク + テンプレート前提）。

## References

- 要件・フェーズ計画: [docs/poc-plan.md](docs/poc-plan.md)
- 意思決定記録（ADR）: [docs/adr/](docs/adr/)
- 進行管理（現在地・次アクション）: [PROGRESS.md](PROGRESS.md)
- デザインシステム: [DESIGN.md](DESIGN.md)
