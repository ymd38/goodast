# goodast

UI起点・初心者向けのOSS Web脆弱性スキャナー（DAST）。Nuclei v3 Go SDK をラップし、Webアプリの動的検査をブラウザから実行する。

詳細は [CLAUDE.md](CLAUDE.md) と [docs/poc-plan.md](docs/poc-plan.md) を参照。

## アーキテクチャ

3プロセス構成（**API とスキャンワーカーは分離**・ADR-0001）。

- **api/**（Go / Gin）— スキャン受付・サイト/履歴管理・所有確認・ダッシュボードデータ
- **worker/**（Go）— Nuclei SDK を**ここにのみ**隔離（ADR-0002）。`river` でデキューし非同期スキャン
- **web/**（Nuxt 3）— UI 起点の全操作（※未スキャフォールド）

データフロー: web → api（enqueue / river）→ PostgreSQL → worker（dequeue → Nuclei 実行 → findings 保存）→ web（ダッシュボード描画）

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
