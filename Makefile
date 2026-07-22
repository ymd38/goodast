# goodast 開発タスク。`make help` で一覧を表示する。
#
# DB はローカル開発用 docker-compose（service: db / postgres:16-alpine）を使う。
# 接続情報は変数で上書き可能。本番値は必ず環境変数で上書きすること。

# localhost だと IPv6(::1) に解決され得るが、compose の公開は 127.0.0.1 限定のため IPv4 を明示する
DATABASE_URL      ?= postgres://goodast:goodast@127.0.0.1:5432/goodast?sslmode=disable
TEST_DATABASE_URL ?= $(DATABASE_URL)
MIGRATE           ?= migrate
NUCLEI_VERSION    ?= v3.9.0
NUCLEI_TEMPLATES_VERSION ?= v10.4.5
NUCLEI_TEMPLATES_DIR      ?= $(CURDIR)/nuclei-templates
NUCLEI_TEST_TARGET ?= http://localhost:3001
GO_MODULES        := api worker jobs secrets
# 開発専用の暗号鍵は各開発者のローカルにのみ生成する（リポジトリに固定鍵を置かない・ADR-0003）。
# 本番は必ず環境変数 GOODAST_ENCRYPTION_KEY で上書きすること。
DEV_KEY_FILE      := $(CURDIR)/.dev-encryption-key

.DEFAULT_GOAL := help

.PHONY: help
help: ## このヘルプを表示する
	@grep -hE '^[a-zA-Z0-9_./-]+:.*?## ' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---- セットアップ ----
.PHONY: setup
setup: ## Go モジュール依存の取得・git hooks 有効化・web の pnpm install
	@for m in $(GO_MODULES); do echo "==> $$m"; (cd $$m && go mod download); done
	@git config core.hooksPath .githooks && echo "==> git hooks 有効化（.githooks）"
	@echo "==> web" && cd web && pnpm install
	@$(MAKE) nuclei-templates

# ---- DB / マイグレーション ----
.PHONY: db-up
db-up: ## PostgreSQL を docker-compose で起動する
	docker compose up -d db

.PHONY: db-down
db-down: ## PostgreSQL を停止する
	docker compose stop db

.PHONY: db-shell
db-shell: ## PostgreSQL に psql で接続する（コンテナ内の環境変数を使用）
	docker compose exec db sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'

.PHONY: db-clean
db-clean: ## 開発DBのアプリデータ（sites/scans/findings/認証情報/ジョブ）を全削除する（スキーマ・マイグレーションは保持）
	@echo "⚠️  DB のアプリデータを全削除します（sites / scan_credentials / scans / findings / river_job）"
	docker compose exec -T db sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -v ON_ERROR_STOP=1 \
		-c "TRUNCATE sites, scan_credentials, scans, findings RESTART IDENTITY CASCADE;" \
		-c "TRUNCATE river_job;"'
	@echo "✅ クリーニング完了（スキーマは保持。再スキャンは make dev-worker が動いていれば即受付可能）"

.PHONY: migrate
migrate: ## マイグレーションを適用する（up）
	$(MIGRATE) -path migrations -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## マイグレーションを1つ戻す（down 1）
	$(MIGRATE) -path migrations -database "$(DATABASE_URL)" down 1

.PHONY: sqlc
sqlc: ## sqlc 生成コードを再生成する（api / worker）
	cd api && sqlc generate
	cd worker && sqlc generate

SWAG_VERSION ?= v1.16.6
.PHONY: swagger
swagger: ## OpenAPI(Swagger) を handler 注釈から再生成する（api/internal/docs）
	cd api && go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) init \
		-g cmd/api/main.go -o internal/docs --parseInternal --parseDependency --quiet

# ---- 開発起動 ----
.PHONY: dev-api
.PHONY: dev-key
dev-key: ## 開発専用の暗号鍵をローカル生成する（未存在時のみ・gitignore 済み）
	@test -f "$(DEV_KEY_FILE)" || { openssl rand -base64 32 | tr -d '\n' > "$(DEV_KEY_FILE)"; echo "generated $(DEV_KEY_FILE)"; }

dev-api: dev-key ## API サーバを起動する（Gin）
	cd api && DATABASE_URL="$(DATABASE_URL)" GOODAST_ENCRYPTION_KEY="$$(cat '$(DEV_KEY_FILE)')" go run ./cmd/api

.PHONY: dev-worker
dev-worker: dev-key ## スキャンワーカーを起動する（Nuclei SDK 隔離）
	cd worker && DATABASE_URL="$(DATABASE_URL)" GOODAST_ENCRYPTION_KEY="$$(cat '$(DEV_KEY_FILE)')" \
		NUCLEI_TEMPLATES_DIR="$(NUCLEI_TEMPLATES_DIR)" NUCLEI_TEMPLATES_VERSION="$(NUCLEI_TEMPLATES_VERSION)" \
		go run ./cmd/worker

.PHONY: dev-web
dev-web: ## web 開発サーバを起動する（Nuxt・/api は :8080 へ devProxy）
	cd web && pnpm dev

# ---- テスト / Lint ----
.PHONY: test
test: test-api test-worker test-web ## 全テスト（Go race + web lint/type-check/vitest）

.PHONY: test-api
test-api: ## api ユニットテスト
	cd api && go test ./... -race

.PHONY: test-worker
test-worker: ## worker ユニットテスト
	cd worker && go test ./... -race

.PHONY: test-web
test-web: ## web テスト（lint + type-check + vitest coverage 100% ゲート）
	cd web && pnpm lint && pnpm type-check && pnpm test --run --coverage

.PHONY: test-integration
test-integration: ## 結合テスト（要 DB 起動・migrate 済 / TEST_DATABASE_URL）
	cd api && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -tags=integration ./... -race
	cd worker && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -tags=integration ./... -race

.PHONY: lint
lint: ## golangci-lint（api / worker・CI と同設定）
	cd api && golangci-lint run --timeout=5m
	cd worker && golangci-lint run --timeout=5m

.PHONY: cover
cover: ## worker ユニットカバレッジ（除外: db / cmd / engine/nuclei / engine/discovery/katana / scanjob）
	cd worker && go test -race -covermode=atomic -coverprofile=coverage.out \
		$$(go list ./... | grep -v '/db$$\|/cmd/\|/engine/nuclei$$\|/engine/discovery/katana$$\|/scanjob$$') \
		&& go tool cover -func=coverage.out | grep -E "^total"

# ---- 検証環境（OWASP Juice Shop / 検知精度の検証）----
.PHONY: juiceshop-up
juiceshop-up: ## Juice Shop を起動する（http://localhost:3001・loopback 限定）
	docker compose --profile juiceshop up -d juiceshop

.PHONY: juiceshop-down
juiceshop-down: ## Juice Shop を停止・削除する
	docker compose --profile juiceshop rm -sf juiceshop

.PHONY: nuclei-templates
nuclei-templates: ## nuclei-templates を固定 tag（NUCLEI_TEMPLATES_VERSION）で取得しマーカーを書く
	@if [ "$$(cat '$(NUCLEI_TEMPLATES_DIR)/.goodast-templates-version' 2>/dev/null)" = "$(NUCLEI_TEMPLATES_VERSION)" ]; then \
		echo "==> nuclei-templates $(NUCLEI_TEMPLATES_VERSION) は導入済み（スキップ）"; \
	else \
		set -e; \
		if [ -z '$(NUCLEI_TEMPLATES_DIR)' ]; then echo "NUCLEI_TEMPLATES_DIR is empty" >&2; exit 1; fi; \
		echo "==> nuclei-templates $(NUCLEI_TEMPLATES_VERSION) を取得: $(NUCLEI_TEMPLATES_DIR)"; \
		rm -rf '$(NUCLEI_TEMPLATES_DIR)'; \
		git clone --depth 1 --branch '$(NUCLEI_TEMPLATES_VERSION)' \
			https://github.com/projectdiscovery/nuclei-templates '$(NUCLEI_TEMPLATES_DIR)'; \
		rm -rf '$(NUCLEI_TEMPLATES_DIR)/.git'; \
		printf '%s' '$(NUCLEI_TEMPLATES_VERSION)' > '$(NUCLEI_TEMPLATES_DIR)/.goodast-templates-version'; \
		echo "==> マーカー書き込み完了"; \
	fi

.PHONY: nuclei-scan
nuclei-scan: ## 対象へ実スキャン結合テスト（NUCLEI_TEST_TARGET / NUCLEI_TEST_TAGS）
	cd worker && NUCLEI_TEST_TARGET="$(NUCLEI_TEST_TARGET)" NUCLEI_TEMPLATES_DIR="$(NUCLEI_TEMPLATES_DIR)" \
		go test -tags=integration -v -timeout 8m -run TestNucleiEngineScan ./internal/engine/nuclei/

.PHONY: nuclei-parity
nuclei-parity: ## 検知精度 検証: Nuclei CLI ベースライン vs goodast の欠落ゼロ突合（要 make juiceshop-up）
	cd worker && NUCLEI_TEST_TARGET="$(NUCLEI_TEST_TARGET)" NUCLEI_TEST_TAGS="$(NUCLEI_TEST_TAGS)" NUCLEI_TEMPLATES_DIR="$(NUCLEI_TEMPLATES_DIR)" \
		go test -tags=integration -v -timeout 25m -run TestNucleiCLIParity ./internal/engine/nuclei/

.PHONY: nuclei-auth
nuclei-auth: ## 認証後スキャン検証（§10-3）: ヘッダ注入の到達（決定的）+ 認証カバレッジ縮小なし（要 make juiceshop-up）
	cd worker && NUCLEI_TEST_TARGET="$(NUCLEI_TEST_TARGET)" NUCLEI_TEST_TAGS="$(NUCLEI_TEST_TAGS)" NUCLEI_TEMPLATES_DIR="$(NUCLEI_TEMPLATES_DIR)" \
		go test -tags=integration -v -timeout 30m -run 'TestNucleiHeaderInjection|TestNucleiAuthenticatedCoverage' ./internal/engine/nuclei/

.PHONY: discovery-scan
discovery-scan: ## Katana 探索の integration テスト（要 juiceshop-up。NUCLEI_TEST_TARGET 上書き可）
	cd worker && go test -tags=integration -v -timeout 8m -run TestKatanaCrawl ./internal/engine/discovery/katana/
