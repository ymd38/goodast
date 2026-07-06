# nuclei-templates 固定版取得の配線 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** nuclei-templates を固定 git tag で取得し、worker 起動時に存在・版を検証（fail-fast）、Nuclei SDK を `NUCLEI_TEMPLATES_DIR` で同ディレクトリへ向ける配線を用意する。

**Architecture:** 環境変数 `NUCLEI_TEMPLATES_DIR` 1 本を軸に、取得（Makefile が固定 tag を clone + マーカー書き込み）・検証（worker の純粋関数 `templates.Verify` がマーカーと版を突合し fail-fast）・ロード（Nuclei SDK が native に同 env を解決）を結ぶ。コードから `os.Setenv` はしない（12-factor）。

**Tech Stack:** Go 1.26.4 / GNU Make / Nuclei v3 SDK（worker のみ・ADR-0002）/ testify / slog / go.uber.org/dig

## Global Constraints

- 設定はすべて環境変数から読む。必須変数が未設定なら起動を失敗させる（サイレントなデフォルト起動禁止）。秘密情報は設定ファイルに書かない。
- Nuclei SDK の import は `worker/internal/engine/nuclei` のみ（ADR-0002）。本タスクの新規パッケージ `worker/internal/templates` は SDK 非依存の純粋ロジック。
- エラーは `fmt.Errorf("...: %w", err)` でラップ。`errors.Is` で判定。context は伝播。
- ログは `log/slog`（`fmt.Println` / `log.Printf` 禁止）。平文の秘密ログ禁止。
- 純粋パッケージ（`templates`）は unit カバレッジ C0 100%。テーブル駆動 + `-race`。
- Nuclei SDK バージョンは `go.mod` で固定（v3.9.0）。`go get -u` しない。
- 固定 tag のデフォルト値: `NUCLEI_TEMPLATES_VERSION = v10.4.5`（nuclei v3.9.0 互換・上書き可）。テンプレートディレクトリのデフォルト: `NUCLEI_TEMPLATES_DIR = $(CURDIR)/nuclei-templates`（既に `.gitignore` の `/nuclei-templates/` で除外済み）。
- バージョンマーカーのファイル名: `.goodast-templates-version`（テンプレートディレクトリ直下）。
- 検証コマンド: worker は `cd worker && go test ./... -race` と `golangci-lint run`。

---

### Task 1: `templates.Verify` 検証パッケージ

テンプレート導入状態を検証する純粋ロジック。SDK 非依存・unit 100%。

**Files:**
- Create: `worker/internal/templates/verify.go`
- Test: `worker/internal/templates/verify_test.go`

**Interfaces:**
- Produces:
  - `const MarkerFile = ".goodast-templates-version"`
  - `var ErrTemplatesMissing = errors.New("templates: nuclei-templates missing or version mismatch")`
  - `func Verify(dir, wantVersion string) error` — dir 内の `MarkerFile` を読み、trim 後の内容が `wantVersion`（trim 後）と一致すれば nil。dir 不在・マーカー不在・不一致は `ErrTemplatesMissing` をラップして返す（メッセージに `make nuclei-templates` を含める）。

- [ ] **Step 1: Write the failing test**

`worker/internal/templates/verify_test.go`:
```go
package templates

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeMarker(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, MarkerFile), []byte(content), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
}

func TestVerify(t *testing.T) {
	const want = "v10.4.5"
	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns dir to verify
		wantErr bool
	}{
		{
			name:    "dir missing",
			setup:   func(t *testing.T) string { return filepath.Join(t.TempDir(), "does-not-exist") },
			wantErr: true,
		},
		{
			name:    "marker missing",
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: true,
		},
		{
			name: "version mismatch",
			setup: func(t *testing.T) string {
				d := t.TempDir()
				writeMarker(t, d, "v10.0.0")
				return d
			},
			wantErr: true,
		},
		{
			name: "version match",
			setup: func(t *testing.T) string {
				d := t.TempDir()
				writeMarker(t, d, want)
				return d
			},
			wantErr: false,
		},
		{
			name: "version match with surrounding whitespace",
			setup: func(t *testing.T) string {
				d := t.TempDir()
				writeMarker(t, d, "  "+want+"\n")
				return d
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			err := Verify(dir, want)
			if tt.wantErr {
				if !errors.Is(err, ErrTemplatesMissing) {
					t.Fatalf("Verify() err = %v, want ErrTemplatesMissing", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() err = %v, want nil", err)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd worker && go test ./internal/templates/ -run TestVerify -v`
Expected: FAIL（`undefined: Verify` / `undefined: MarkerFile` / `undefined: ErrTemplatesMissing`）

- [ ] **Step 3: Write minimal implementation**

`worker/internal/templates/verify.go`:
```go
// Package templates は nuclei-templates の導入状態を検証する。SDK 非依存の純粋ロジックに保つ。
//
// テンプレートは make nuclei-templates が固定 git tag で取得し、取得側が MarkerFile に版文字列を
// 書き込む。Nuclei 自身のインストール追跡 JSON は git clone 直取得では更新されないため、この
// 自前マーカーを版の正とする（設計: docs/superpowers/specs/2026-07-06-nuclei-templates-pinning-design.md）。
package templates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MarkerFile は make nuclei-templates が書き込む版マーカーのファイル名。
const MarkerFile = ".goodast-templates-version"

// ErrTemplatesMissing はテンプレートディレクトリが無い／版が一致しないことを表す。
var ErrTemplatesMissing = errors.New("templates: nuclei-templates missing or version mismatch")

// Verify は dir に MarkerFile があり、その内容（trim 後）が wantVersion（trim 後）と一致することを
// 確認する。不在・不一致は ErrTemplatesMissing をラップして返す。
func Verify(dir, wantVersion string) error {
	raw, err := os.ReadFile(filepath.Join(dir, MarkerFile))
	if err != nil {
		return fmt.Errorf("%w: read marker in %q (run `make nuclei-templates`): %w", ErrTemplatesMissing, dir, err)
	}
	got := strings.TrimSpace(string(raw))
	if got != strings.TrimSpace(wantVersion) {
		return fmt.Errorf("%w: installed %q != pinned %q (run `make nuclei-templates`)", ErrTemplatesMissing, got, wantVersion)
	}
	return nil
}
```

> `os.ReadFile` は dir 不在（親ディレクトリ無し）でもマーカー不在（dir はあるがファイル無し）でも
> エラーを返すため、両ケースを 1 経路で扱える（別途 `os.Stat(dir)` は不要・DRY）。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd worker && go test ./internal/templates/ -race -v`
Expected: PASS（5 サブテスト）

- [ ] **Step 5: Verify 100% coverage**

Run: `cd worker && go test -covermode=atomic -coverprofile=/tmp/covt.out ./internal/templates/ && go tool cover -func=/tmp/covt.out | grep -E "verify.go|^total"`
Expected: `Verify 100.0%` / `total: 100.0%`

- [ ] **Step 6: Commit**

```bash
git add worker/internal/templates/verify.go worker/internal/templates/verify_test.go
git commit -m "feat(worker): nuclei-templates 導入状態を検証する templates.Verify を追加する"
```

（コミットメッセージ本文末尾に付与:
`Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>` と
`Claude-Session: https://claude.ai/code/session_012ZYDjr15MCdtw7sCM262NF`）

---

### Task 2: worker config に templates 用 env を追加

`NUCLEI_TEMPLATES_DIR` / `NUCLEI_TEMPLATES_VERSION` を必須 env として読む（値の読取のみ・FS 検証はしない）。

**Files:**
- Modify: `worker/internal/config/config.go`
- Modify: `worker/internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config` に `NucleiTemplatesDir string` と `NucleiTemplatesVersion string` フィールド。両方 env 必須（欠落で `Load` がエラー）。

- [ ] **Step 1: Update the test first (add required-env cases + extend base/clearEnv)**

`worker/internal/config/config_test.go` を編集する。

(a) env をクリアするヘルパのキー一覧（現在 `"DATABASE_URL", "GOODAST_ENCRYPTION_KEY", "WORKER_HEALTH_ADDR", "LOG_LEVEL", ...` を列挙している箇所）に `"NUCLEI_TEMPLATES_DIR"` と `"NUCLEI_TEMPLATES_VERSION"` を追加する。

(b) `base` ヘルパ（必須変数を満たす env を返す・現在 `DATABASE_URL` と `GOODAST_ENCRYPTION_KEY` を入れている）に 2 つの必須値を追加する:
```go
env := map[string]string{
	"DATABASE_URL":             validURL,
	"GOODAST_ENCRYPTION_KEY":   validKey,
	"NUCLEI_TEMPLATES_DIR":     "/tmp/nuclei-templates",
	"NUCLEI_TEMPLATES_VERSION": "v10.4.5",
}
```

(c) テーブルに欠落エラーケースを 2 つ追加する（`base` から各キーを外す形。`base` はキー追加後の全必須を含むので、1 つ削って wantErr にする）:
```go
{name: "missing NUCLEI_TEMPLATES_DIR", env: baseWithout("NUCLEI_TEMPLATES_DIR"), wantErr: true},
{name: "missing NUCLEI_TEMPLATES_VERSION", env: baseWithout("NUCLEI_TEMPLATES_VERSION"), wantErr: true},
```
`baseWithout` ヘルパを test 内に追加（`base` からキーを 1 つ削除したマップを返す）:
```go
baseWithout := func(drop string) map[string]string {
	m := base(nil)
	delete(m, drop)
	return m
}
```
> 既存の成功ケースは `base(...)` を使っているため、`base` に 2 キーを足せば自動的に必須が満たされ、
> 回帰しない。成功時のフィールド検証に以下を追加してよい（任意）:
> `if cfg.NucleiTemplatesDir != tt.env["NUCLEI_TEMPLATES_DIR"] { t.Errorf(...) }`

- [ ] **Step 2: Run test to verify it fails**

Run: `cd worker && go test ./internal/config/ -run TestLoad -v`
Expected: FAIL（`cfg.NucleiTemplatesDir` 未定義でコンパイルエラー、または missing ケースが通らない）

- [ ] **Step 3: Add fields + required reads in config.go**

`worker/internal/config/config.go`:

(a) `Config` struct にフィールド追加:
```go
	NucleiTemplatesDir     string // NUCLEI_TEMPLATES_DIR。SDK が読むのと同一。templates.Verify で検証する。
	NucleiTemplatesVersion string // NUCLEI_TEMPLATES_VERSION。固定 tag。マーカーと突合する。
```

(b) `Load` 内、`encKey` の必須チェックの後あたりに追加:
```go
	templatesDir := os.Getenv("NUCLEI_TEMPLATES_DIR")
	if templatesDir == "" {
		return nil, fmt.Errorf("NUCLEI_TEMPLATES_DIR is required")
	}
	templatesVersion := os.Getenv("NUCLEI_TEMPLATES_VERSION")
	if templatesVersion == "" {
		return nil, fmt.Errorf("NUCLEI_TEMPLATES_VERSION is required")
	}
```

(c) 返す `&Config{...}` に 2 フィールドを追加:
```go
		NucleiTemplatesDir:     templatesDir,
		NucleiTemplatesVersion: templatesVersion,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd worker && go test ./internal/config/ -race -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add worker/internal/config/config.go worker/internal/config/config_test.go
git commit -m "feat(worker): config に NUCLEI_TEMPLATES_DIR/VERSION を必須 env として追加する"
```

---

### Task 3: worker 起動シーケンスに fail-fast 検証を挿入

`config.Load` 後・DI 配線前に `templates.Verify` を呼び、失敗で起動中断。

**Files:**
- Modify: `worker/cmd/worker/main.go`

**Interfaces:**
- Consumes: `config.Config.NucleiTemplatesDir` / `.NucleiTemplatesVersion`（Task 2）, `templates.Verify`（Task 1）

- [ ] **Step 1: Insert Verify call after logger setup**

`worker/cmd/worker/main.go` の import に追加:
```go
	"github.com/ymd38/goodast/worker/internal/templates"
```
`run()` 内、`slog.SetDefault(logger)` の直後（dig `c := dig.New()` の前）に挿入:
```go
	// fail-fast: nuclei-templates が固定版で導入済みでなければ scan は必ず失敗するため起動を中断する。
	// テンプレートは make nuclei-templates が取得し、SDK は NUCLEI_TEMPLATES_DIR を native に読む。
	if err := templates.Verify(cfg.NucleiTemplatesDir, cfg.NucleiTemplatesVersion); err != nil {
		return fmt.Errorf("verify nuclei-templates: %w", err)
	}
	logger.Info("nuclei-templates verified", "dir", cfg.NucleiTemplatesDir, "version", cfg.NucleiTemplatesVersion)
```

- [ ] **Step 2: Verify build**

Run: `cd worker && go build ./...`
Expected: 成功

- [ ] **Step 3: Confirm the worker unit suite still passes**

Run: `cd worker && go test ./... -race`
Expected: PASS（`cmd/worker` は no test files。既存パッケージ緑）

- [ ] **Step 4: Commit**

```bash
git add worker/cmd/worker/main.go
git commit -m "feat(worker): 起動時に nuclei-templates の導入版を検証し fail-fast する"
```

---

### Task 4: Makefile を固定 tag 取得へ書き換え + env 伝播

`nuclei-templates` を `git clone --branch` 方式に変え、`setup` 連携・マーカー書き込み・dev/test への env 伝播を行う。

**Files:**
- Modify: `Makefile`
- Modify: `.env.example`

**Interfaces:**
- Produces: `make nuclei-templates` が `$(NUCLEI_TEMPLATES_DIR)` に固定 tag のテンプレートと `.goodast-templates-version` マーカーを用意する。

- [ ] **Step 1: Add variables**

`Makefile` の変数定義部（`NUCLEI_VERSION ?= v3.9.0` の近く）に追加:
```makefile
NUCLEI_TEMPLATES_VERSION ?= v10.4.5
NUCLEI_TEMPLATES_DIR      ?= $(CURDIR)/nuclei-templates
```

- [ ] **Step 2: Rewrite the nuclei-templates target**

既存の `nuclei-templates` ターゲット（`go run ... -update-templates` の 2 行）を置換:
```makefile
.PHONY: nuclei-templates
nuclei-templates: ## nuclei-templates を固定 tag（NUCLEI_TEMPLATES_VERSION）で取得しマーカーを書く
	@if [ "$$(cat '$(NUCLEI_TEMPLATES_DIR)/.goodast-templates-version' 2>/dev/null)" = "$(NUCLEI_TEMPLATES_VERSION)" ]; then \
		echo "==> nuclei-templates $(NUCLEI_TEMPLATES_VERSION) は導入済み（スキップ）"; \
	else \
		echo "==> nuclei-templates $(NUCLEI_TEMPLATES_VERSION) を取得: $(NUCLEI_TEMPLATES_DIR)"; \
		rm -rf '$(NUCLEI_TEMPLATES_DIR)'; \
		git clone --depth 1 --branch '$(NUCLEI_TEMPLATES_VERSION)' \
			https://github.com/projectdiscovery/nuclei-templates '$(NUCLEI_TEMPLATES_DIR)'; \
		rm -rf '$(NUCLEI_TEMPLATES_DIR)/.git'; \
		printf '%s' '$(NUCLEI_TEMPLATES_VERSION)' > '$(NUCLEI_TEMPLATES_DIR)/.goodast-templates-version'; \
		echo "==> マーカー書き込み完了"; \
	fi
```
> 冪等: マーカーが固定版と一致すれば再 clone しない。`.git` を消して実行時 git 依存と容量を削る。
> `printf '%s'`（改行なし）で書くが、`Verify` は trim するため末尾改行有無に依存しない。

- [ ] **Step 3: Wire setup to call it**

`setup` ターゲット本体（`go mod download` ループ・git hooks・pnpm install の後）に 1 行追加:
```makefile
	@$(MAKE) nuclei-templates
```
（`setup:` の最後の行の後に追記。`.PHONY: setup` はそのまま）

- [ ] **Step 4: Propagate env to worker/dev + nuclei test targets**

以下のターゲットのコマンド行に `NUCLEI_TEMPLATES_DIR` を渡す（SDK が読む）。worker 起動系（`dev-worker`）には `NUCLEI_TEMPLATES_VERSION` も渡す（config が必須にするため）。

`dev-worker`:
```makefile
	cd worker && DATABASE_URL="$(DATABASE_URL)" GOODAST_ENCRYPTION_KEY="$$(cat '$(DEV_KEY_FILE)')" \
		NUCLEI_TEMPLATES_DIR="$(NUCLEI_TEMPLATES_DIR)" NUCLEI_TEMPLATES_VERSION="$(NUCLEI_TEMPLATES_VERSION)" \
		go run ./cmd/worker
```

`nuclei-scan` / `nuclei-parity` / `nuclei-auth`: 各 `cd worker && NUCLEI_TEST_TARGET=...` の環境に `NUCLEI_TEMPLATES_DIR="$(NUCLEI_TEMPLATES_DIR)"` を追加する（integration テストが SDK 経由でテンプレートを読むため）。例（`nuclei-scan`）:
```makefile
	cd worker && NUCLEI_TEST_TARGET="$(NUCLEI_TEST_TARGET)" NUCLEI_TEMPLATES_DIR="$(NUCLEI_TEMPLATES_DIR)" \
		go test -tags=integration -v -timeout 8m -run TestNucleiEngineScan ./internal/engine/nuclei/
```
`nuclei-parity` / `nuclei-auth` も同様に `NUCLEI_TEMPLATES_DIR="$(NUCLEI_TEMPLATES_DIR)"` を既存 env の並びに追加する。

- [ ] **Step 5: Document env in .env.example**

`.env.example` に追記（テンプレート配布セクション。既存の書式に合わせてコメント + 変数）:
```bash
# Nuclei テンプレート（固定版）。make nuclei-templates が取得し worker 起動時に版を検証する。
NUCLEI_TEMPLATES_DIR=./nuclei-templates
NUCLEI_TEMPLATES_VERSION=v10.4.5
```

- [ ] **Step 6: Verify Makefile syntax + help lists the target**

Run: `make -n nuclei-templates && make help | grep -E "nuclei-templates|setup"`
Expected: `make -n` がエラーなくコマンドを展開表示し、`help` に両ターゲットが出る（`make -n` は実行せずコマンドを表示するだけなので clone は走らない）

- [ ] **Step 7: Commit**

```bash
git add Makefile .env.example
git commit -m "chore(infra): nuclei-templates を固定 tag 取得にし env を配線する"
```

---

### Task 5: 実取得の疎通確認 + PROGRESS/メモ更新

固定 tag の実クローンと worker 起動 fail-fast を 1 度確認し、進行管理を更新する。

**Files:**
- Modify: `PROGRESS.md`

- [ ] **Step 1: Confirm the pinned tag exists on the remote**

Run: `git ls-remote --tags --refs https://github.com/projectdiscovery/nuclei-templates 'refs/tags/v10.4.5'`
Expected: 1 行返る（tag が実在）。もし空なら、`git ls-remote --tags ... | sort -V | tail` で最新の実在 tag を確認し、Makefile と .env.example と config_test の既定値を合わせて修正する（3 箇所同期）。

- [ ] **Step 2: Run the real acquisition (idempotent check)**

Run:
```bash
cd /Users/hirokazuyamada/go/src/goodast
make nuclei-templates
cat nuclei-templates/.goodast-templates-version   # -> v10.4.5
make nuclei-templates                              # 2 回目はスキップ表示
```
Expected: 1 回目で clone + マーカー、2 回目は「導入済み（スキップ）」。`nuclei-templates/` に .yaml テンプレートが多数存在し、`.git` は無い。

- [ ] **Step 3: Confirm fail-fast + success at worker startup**

fail-fast（版不一致）:
```bash
cd worker && DATABASE_URL="postgres://goodast:goodast@127.0.0.1:5432/goodast?sslmode=disable" \
	GOODAST_ENCRYPTION_KEY="$(cat ../.dev-encryption-key 2>/dev/null || echo AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=)" \
	NUCLEI_TEMPLATES_DIR="$(pwd)/../nuclei-templates" NUCLEI_TEMPLATES_VERSION="v0.0.0-wrong" \
	go run ./cmd/worker 2>&1 | head -5
```
Expected: `verify nuclei-templates: ... version mismatch ...` を出して非ゼロ終了（fail-fast）。
> 鍵は形式が通れば良い（検証は cipher 構築時）。DB 未起動でも templates 検証は cipher/pool より前なので mismatch で先に落ちることを確認する。正しい版（`v10.4.5`）に変えると templates 検証は通過し、以降の初期化に進む（そこで DB 等が必要）。この確認は「templates 検証が起動シーケンスの正しい位置で効く」ことの確認に留める。

- [ ] **Step 4: Update PROGRESS.md**

`PROGRESS.md` の「ADR-0002 の持ち越し / 留意点」内の「**nuclei-templates の取得は未実装**」の項を、固定 tag 取得・worker 起動時検証・SDK ディレクトリ明示が実装済みである旨に更新する。「現在地スナップショット」に本作業の完了を 1 行追加する。既定 tag（`v10.4.5`）と env（`NUCLEI_TEMPLATES_DIR`/`_VERSION`）に触れる。

- [ ] **Step 5: Commit**

```bash
git add PROGRESS.md
git commit -m "docs: nuclei-templates 固定版取得の配線完了を記録する"
```

---

## Self-Review

**Spec coverage:**
- 検証パッケージ `templates.Verify`（マーカー突合・fail-fast の中核）→ Task 1 ✅
- worker config に env 2 追加（必須）→ Task 2 ✅
- worker 起動シーケンスに fail-fast 挿入 → Task 3 ✅
- Makefile 固定 tag 取得・マーカー書き込み・冪等・setup 連携・env 伝播・.gitignore（既存で充足）→ Task 4 ✅
- .env.example ドキュメント → Task 4 Step 5 ✅
- SDK へのディレクトリ明示（`NUCLEI_TEMPLATES_DIR` を env で渡し SDK native 解決・os.Setenv しない）→ Task 4 Step 4（env 伝播）で担保、nuclei.go 変更不要（spec の通り）✅
- 実取得疎通・版 tag 実在確認・PROGRESS 更新 → Task 5 ✅

**Placeholder scan:** プレースホルダなし。各コード step は実コード/実コマンドを提示。Task 5 の tag 確認は「実在しなければ最新へ合わせる」具体手順を明記。

**Type consistency:**
- `templates.Verify(dir, wantVersion string) error` / `templates.MarkerFile` / `templates.ErrTemplatesMissing` — Task 1 定義、Task 3 で使用（`Verify` のみ）。
- `config.Config.NucleiTemplatesDir` / `.NucleiTemplatesVersion` — Task 2 定義、Task 3 で使用。
- env 名 `NUCLEI_TEMPLATES_DIR` / `NUCLEI_TEMPLATES_VERSION`、既定値 `v10.4.5` / `$(CURDIR)/nuclei-templates`、マーカー名 `.goodast-templates-version` — Task 1/2/4/5 で一貫。

**ビルド順序:** Task 1→2→3 は worker がビルド可能な状態を保つ（Task 3 で `templates` と config の新フィールドを使うが、両方 Task 1/2 で先に入る）。Task 4/5 はコード非依存（Makefile/docs/確認）。
