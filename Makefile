# goodast 開発タスク。`make help` で一覧を表示する。
#
# DB はローカル開発用 docker-compose（service: db / postgres:16-alpine）を使う。
# 接続情報は変数で上書き可能。本番値は必ず環境変数で上書きすること。

DATABASE_URL      ?= postgres://goodast:goodast@localhost:5432/goodast?sslmode=disable
TEST_DATABASE_URL ?= $(DATABASE_URL)
MIGRATE           ?= migrate
NUCLEI_VERSION    ?= v3.9.0
NUCLEI_TEST_TARGET ?= http://localhost:3001
GO_MODULES        := api worker jobs

.DEFAULT_GOAL := help

.PHONY: help
help: ## このヘルプを表示する
	@grep -hE '^[a-zA-Z0-9_./-]+:.*?## ' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---- セットアップ ----
.PHONY: setup
setup: ## Go モジュール依存を取得する（web は未スキャフォールド）
	@for m in $(GO_MODULES); do echo "==> $$m"; (cd $$m && go mod download); done

# ---- DB / マイグレーション ----
.PHONY: db-up
db-up: ## PostgreSQL を docker-compose で起動する
	docker compose up -d db

.PHONY: db-down
db-down: ## PostgreSQL を停止する
	docker compose stop db

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

# ---- 開発起動 ----
.PHONY: dev-api
dev-api: ## API サーバを起動する（Gin）
	cd api && DATABASE_URL="$(DATABASE_URL)" go run ./cmd/api

.PHONY: dev-worker
dev-worker: ## スキャンワーカーを起動する（Nuclei SDK 隔離）
	cd worker && DATABASE_URL="$(DATABASE_URL)" go run ./cmd/worker

.PHONY: dev-web
dev-web: ## (TODO) web は未スキャフォールド
	@echo "web/ は未スキャフォールドです（package.json 追加後に有効化）"

# ---- テスト / Lint ----
.PHONY: test
test: test-api test-worker ## 全 Go ユニットテスト（race）

.PHONY: test-api
test-api: ## api ユニットテスト
	cd api && go test ./... -race

.PHONY: test-worker
test-worker: ## worker ユニットテスト
	cd worker && go test ./... -race

.PHONY: test-integration
test-integration: ## 結合テスト（要 DB 起動・migrate 済 / TEST_DATABASE_URL）
	cd api && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -tags=integration ./... -race
	cd worker && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -tags=integration ./... -race

.PHONY: lint
lint: ## golangci-lint（api / worker・CI と同設定）
	cd api && golangci-lint run --timeout=5m
	cd worker && golangci-lint run --timeout=5m

.PHONY: cover
cover: ## worker ユニットカバレッジ（除外: db / cmd / engine/nuclei / scanjob）
	cd worker && go test -race -covermode=atomic -coverprofile=coverage.out \
		$$(go list ./... | grep -v '/db$$\|/cmd/\|/engine/nuclei$$\|/scanjob$$') \
		&& go tool cover -func=coverage.out | grep -E "^total"

# ---- 検証環境（OWASP Juice Shop / 検知精度の検証）----
.PHONY: juiceshop-up
juiceshop-up: ## Juice Shop を起動する（http://localhost:3001・loopback 限定）
	docker compose --profile juiceshop up -d juiceshop

.PHONY: juiceshop-down
juiceshop-down: ## Juice Shop を停止・削除する
	docker compose --profile juiceshop rm -sf juiceshop

.PHONY: nuclei-templates
nuclei-templates: ## nuclei-templates を固定版で取得・更新する（版は NUCLEI_VERSION 変数）
	go run github.com/projectdiscovery/nuclei/v3/cmd/nuclei@$(NUCLEI_VERSION) -update-templates

.PHONY: nuclei-scan
nuclei-scan: ## 対象へ実スキャン結合テスト（NUCLEI_TEST_TARGET / NUCLEI_TEST_TAGS）
	cd worker && NUCLEI_TEST_TARGET="$(NUCLEI_TEST_TARGET)" \
		go test -tags=integration -v -timeout 8m -run TestNucleiEngineScan ./internal/engine/nuclei/

.PHONY: nuclei-parity
nuclei-parity: ## 検知精度 検証: Nuclei CLI ベースライン vs goodast の欠落ゼロ突合（要 make juiceshop-up）
	cd worker && NUCLEI_TEST_TARGET="$(NUCLEI_TEST_TARGET)" NUCLEI_TEST_TAGS="$(NUCLEI_TEST_TAGS)" \
		go test -tags=integration -v -timeout 25m -run TestNucleiCLIParity ./internal/engine/nuclei/
