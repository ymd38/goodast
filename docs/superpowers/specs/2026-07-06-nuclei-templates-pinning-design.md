# nuclei-templates 固定版取得の配線 — 設計

最終更新: 2026-07-06

PROGRESS.md「ADR-0002 の持ち越し」/ 企画書 §12「テンプレート配布」の対応。nuclei-templates
（SDK が scan 時に参照する脆弱性テンプレート集）を**固定バージョンで取得**し、worker 起動時に
存在・版を検証（fail-fast）、Nuclei SDK をそのディレクトリに向ける配線を用意する。

## 背景と課題

- 現状 `make nuclei-templates` は `nuclei -update-templates` で**最新**を取得するだけ。バージョン固定
  （企画書 §12）が実現できていない。予期しないテンプレート更新で検知挙動が変わり得る。
- worker はテンプレート導入を前提にするが、**不在でも起動してしまい**、scan 実行時に初めて失敗する
  （または SDK が暗黙にダウンロードを試みる）。12-factor の「サイレントなデフォルト起動禁止」に反する。
- Nuclei SDK にテンプレートディレクトリを明示していない（既定 catalog 依存）。

## 決定事項（承認済み）

1. **スコープ**: setup 取得 ＋ worker 起動時検証 ＋ SDK へのディレクトリ明示、の三点。
2. **バージョン固定**: nuclei-templates の**正規 git tag を厳密ピン**（`git clone --branch`）。
3. **起動挙動**: テンプレート不在・版不一致は **fail-fast**（起動を失敗させる）。

## アーキテクチャ: env 変数 1 本で 3 者を繋ぐ

`NUCLEI_TEMPLATES_DIR` は **Nuclei SDK が native に読む環境変数**（`pkg/catalog/config` の
`NucleiTemplatesDirEnv`）。これを軸に取得・検証・ロードを 1 つのパスに束ねる。

```
make nuclei-templates  → 固定 tag を DIR に git clone + バージョンマーカー書き込み
        ↓ (同じ DIR)
worker 起動時          → DIR 存在 + マーカー == 固定版 を検証（不一致で fail-fast）
        ↓ (同じ DIR)
Nuclei SDK             → NUCLEI_TEMPLATES_DIR を native に解決しテンプレートをロード
```

**`os.Setenv` をコードでやらない**（12-factor）。`NUCLEI_TEMPLATES_DIR` は環境で与える（dev は
Makefile、prod は将来の Dockerfile の ENV）。worker はそれを読んで検証し、SDK は同じ変数を
native に解決する。「SDK への明示」は「必須・検証済み config として与える（サイレントデフォルト
禁止）」で担保する。

## コンポーネント

### 1. Makefile

- 変数追加:
  - `NUCLEI_TEMPLATES_VERSION ?= v10.4.5`（nuclei v3.9.0 互換の実在 tag。上書き可）
  - `NUCLEI_TEMPLATES_DIR ?= $(CURDIR)/nuclei-templates`（repo 直下・既に `.gitignore` の
    `/nuclei-templates/` で除外済み）
- `nuclei-templates` ターゲットを書き換え（`-update-templates` を廃し固定 tag 取得に）:
  - 既存 DIR のマーカーが `NUCLEI_TEMPLATES_VERSION` と一致 → スキップ（冪等）
  - それ以外 → DIR を作り直し `git clone --depth 1 --branch $(NUCLEI_TEMPLATES_VERSION)
    https://github.com/projectdiscovery/nuclei-templates $(NUCLEI_TEMPLATES_DIR)`
  - clone 後、`$(NUCLEI_TEMPLATES_DIR)/.goodast-templates-version` に版文字列を書き込む
  - clone に含まれる `.git` は削除（実行時 git 依存を残さない・容量削減）
- `setup` から `nuclei-templates` を呼ぶ（`go mod download` 等と並ぶ一括セットアップ）
- テンプレートを要する既存ターゲットに env を渡す: `dev-worker` / `nuclei-scan` /
  `nuclei-parity` / `nuclei-auth` に `NUCLEI_TEMPLATES_DIR`（＝SDK が読む）と、worker 起動系には
  `NUCLEI_TEMPLATES_VERSION` も渡す

### 2. `worker/internal/templates`（新規パッケージ）

テンプレート導入状態の検証を担う純粋ロジック。

```go
package templates

// ErrTemplatesMissing はテンプレートディレクトリが無い／版が一致しないことを表す。
var ErrTemplatesMissing = errors.New("templates: nuclei-templates missing or version mismatch")

// markerFile は make nuclei-templates が書き込む版マーカーのファイル名。
const markerFile = ".goodast-templates-version"

// Verify は dir にマーカーファイルがあり、その内容が wantVersion と一致することを確認する。
// 不在・不一致は ErrTemplatesMissing をラップして返す（"make nuclei-templates を先に実行" を含む）。
func Verify(dir, wantVersion string) error
```

- FS 検証（ディレクトリ存在・マーカー読取・trim 比較）のみ。SDK 非依存。
- **unit 100%**（`t.TempDir()` で: dir 不在 / マーカー不在 / 版不一致 / 一致 の全分岐）。

### 3. `worker/internal/config`

- `Config` に `NucleiTemplatesDir string` / `NucleiTemplatesVersion string` を追加。
- `Load` で両 env を**必須**として読む（欠落はエラー・サイレントデフォルト禁止）。
  値の読取のみ。**FS 検証は行わない**（責務分離。`templates.Verify` に委ねる）。
- config unit テストに「両 env 欠落 → エラー」ケースを追加。

### 4. `worker/cmd/worker/main.go`

- `config.Load` 成功後、river/engine 配線の**前**に:
  ```go
  if err := templates.Verify(cfg.NucleiTemplatesDir, cfg.NucleiTemplatesVersion); err != nil {
      // fail-fast: テンプレート未導入では scan が必ず失敗するため起動を中断する
      return err // ログに "make nuclei-templates を先に実行" を含める
  }
  ```
- `NUCLEI_TEMPLATES_DIR` はプロセス環境に既に在る前提（Makefile / Dockerfile が設定）。
  SDK が native に解決するため、engine 配線コード（`nuclei.go`）の変更は不要。

## 検証マーカー方式（なぜ SDK の版追跡 JSON を使わないか）

Nuclei 自身のインストール追跡 JSON（`nuclei-templates-version`）は nuclei の installer 経由でのみ
更新される。本設計は `git clone` で直接取得するため、その JSON は更新されない。よって**取得側
（make）が書く `.goodast-templates-version` マーカー**を版の正とし、worker がそれを突合する。
シンプル・自己完結・実行時 git 非依存。

## 影響ファイル

| 対象 | 変更 |
|---|---|
| `Makefile` | 変数 2 追加 / `nuclei-templates` 書き換え / `setup` 連携 / dev・test ターゲットへ env 伝播 |
| `worker/internal/templates/verify.go`（新規） | `Verify` / `ErrTemplatesMissing` / マーカー定数 |
| `worker/internal/templates/verify_test.go`（新規） | 全分岐 unit（temp dir） |
| `worker/internal/config/config.go` | env 2 追加（必須読取） |
| `worker/internal/config/config_test.go` | 欠落エラーケース追加 |
| `worker/cmd/worker/main.go` | `templates.Verify` を起動シーケンスに挿入（fail-fast） |
| `.env.example` | `NUCLEI_TEMPLATES_DIR` / `NUCLEI_TEMPLATES_VERSION` を追記 |

## テスト方針

- `templates.Verify`: テーブル駆動 unit・**C0 100%**（dir 不在／マーカー不在／版不一致／一致）。
- `config.Load`: 新 env の必須化（欠落→エラー）を既存テーブルに追加。
- Makefile `nuclei-templates`: 実クローンは重く CI 常時実行しない（手動確認）。冪等性（2 回実行で
  再 clone しない）はローカルで確認。
- `make test-worker` パス・lint 0 issues 維持。

## 実装時に確定する点

- `NUCLEI_TEMPLATES_VERSION` の既定 tag: 設計時点の最新は `v10.4.5`。実装時に `git ls-remote
  --tags` で最終確認し、nuclei v3.9.0 と組み合わせて engine integration（`make nuclei-scan` 等）が
  走ることを 1 度確認する（テンプレ導入環境がある場合）。
- Nuclei SDK が `NUCLEI_TEMPLATES_DIR` のテンプレートを（installer 追跡 JSON 無しで）ロードし、
  `DisableUpdateCheck()` 下で暗黙ダウンロードを試みないことを integration で確認する。

## 非対象（YAGNI）

- Dockerfile への同梱（Dockerfile 3 本は別タスク。env でパスを渡す設計のため Docker 化時は
  `ENV NUCLEI_TEMPLATES_DIR=...` 一行で接続できる）。
- 起動時自動ダウンロード（fail-fast を選択済み）。
- SDK のテンプレート署名検証経路（git clone 直取得のため対象外・マーカーで版を担保）。
- レート等の env 化（本タスクと無関係）。
