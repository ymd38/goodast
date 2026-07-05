# web (Nuxt 3) スキャフォールド & ダッシュボード — 設計

日付: 2026-07-04
ステータス: 承認済み（ブレインストーミング経由）
スコープ: PoC Phase 1 の frontend 第1弾。7画面のうち「土台 + ダッシュボード（PoC の中心・§6-6）」のみ。

## 背景 / ゴール

- backend の PoC 主要 API は出揃った（PR #18 まで）。`api/internal/docs/swagger.yaml` が API 契約の正。
- web/ は骨格のみ（`tokens.css` / `CLAUDE.md` / 空の `pages/` `components/dashboard/`）。package.json 不在で CI の frontend / pnpm-audit ジョブはコメントアウト中。
- 本設計のゴール: **サイト一覧 → サイト別ダッシュボード（スコア + Chart.js 遷移）を閲覧できる**状態にする。サイト登録・スキャン実行等の操作系画面は次PR以降。

## PR 分割（1PR = 1目的）

| PR | ブランチ | 内容 |
|---|---|---|
| A | `chore/0019-web-scaffold` | Nuxt 3 スキャフォールド + 型付き API クライアント生成 + CI 有効化。画面はレイアウト骨格のみ |
| B | `feat/0020-dashboard` | サイト一覧ページ + サイト別ダッシュボード（スコアカード + Chart.js） |

PR #18（swagger.yaml）はマージ済みのため、A は最新 main から分岐する。

## PR A: スキャフォールド

### スタック（frontend.md 準拠）

- Nuxt 3（TypeScript strict）+ pnpm
- Tailwind CSS v4（`@tailwindcss/vite`）
- Vitest + @nuxt/test-utils、ESLint（`@nuxt/eslint`）
- Chart.js は PR B で追加（`pnpm add chart.js`。CDN 不使用）

### デザイントークン統合

- `assets/css/main.css`: `@import "tailwindcss"` + `tokens.css` を取り込み、`@theme` で `--color-canvas` 等を Tailwind ユーティリティ（`bg-canvas` / `text-body` / `border-hairline` 等のセマンティッククラス）へマップする。
- 生 hex は書かない。トークン値の正は `tokens.css` のまま（`@theme` は `var()` 参照）。
- フォント: `@fontsource` の Inter（weight 700 / 300 のみ・セルフホスト）。`--font-display` のフォールバックとして機能させる。

### 型付き API クライアント

- パイプライン: `api/internal/docs/swagger.yaml`（Swagger 2.0）→ `swagger2openapi` で OpenAPI 3 に変換 → `openapi-typescript` で `web/types/api.d.ts` を生成。
  - **注意: openapi-typescript は OpenAPI 3.x のみ対応。** swaggo 出力が 2.0 のため変換段が必須。
- ランタイムは `openapi-fetch`（生成型をそのまま消費する軽量 fetch）。
- 生成コマンド: `pnpm gen:api`（web/ 内）。生成物 `types/api.d.ts` はコミットする（backend の sqlc 生成物と同じ扱い・手編集禁止）。
- API 変更時の再生成は当面手動（`make swagger` → `pnpm gen:api`）。乖離は TypeScript コンパイルエラーで検出される。

### API 接続

- `runtimeConfig.public.apiBase`（既定 `/api`）。
- 開発時は nitro `devProxy` で `/api` → `http://localhost:8080`（Gin 側 CORS 変更不要）。
- 本番はリバースプロキシで同一オリジン配信を前提（PoC 範囲外）。

### レイアウト骨格

- `layouts/default.vue`: 黒 canvas。トップナビ（GOODAST ワードマーク + M ストライプディバイダ `--gradient-m-stripe`）。DESIGN.md の `top-nav` 仕様（64px / canvas 背景）。
- `pages/index.vue` はプレースホルダ（PR B で実装）。

### ビルド / CI 配線

- Makefile: `dev-web`（`pnpm dev`）/ `test-web`（`pnpm lint` + `pnpm type-check` + `pnpm test --run`）を実体化。`setup` に pnpm install を追加。
- `.github/workflows/ci.yml` の frontend ジョブ、`security-scan.yml` の pnpm-audit ジョブのコメントアウトを解除。
- Vitest カバレッジ設定: 設定ファイル・`*.d.ts`・`assets/` を除外。

## PR B: サイト一覧 & ダッシュボード

### ページ構成

- **`pages/index.vue` — サイト一覧**: `GET /sites`（`siteResponse[]`）。カード（`surface-card` / 角丸0）にサイト名・base_url・`ownership_verified` バッジ。クリックで `/sites/[id]` へ。0件時は空状態（API 経由登録の案内文）。
- **`pages/sites/[id].vue` — ダッシュボード**: `GET /sites/:id`（サイト名）+ `GET /sites/:id/dashboard`（`DashboardData`）。§6-6 の3層構成:

| 層 | コンポーネント | 内容 |
|---|---|---|
| 上段（状態） | `ScoreCard.vue` | `latest.score` 大数字 + Band 色 + `latest.delta`（`+5↑` / `-12↓`、null は非表示）。`latest: null` は「スキャン未実行」 |
| 上段（状態） | `SeverityCountCards.vue` | `latest.counts` の重大度別サマリカード列（spec-cell 風） |
| 中段（遷移） | `ScoreTrendChart.vue` | `history` のスコア折れ線。**history < 2 は「データ不足」空表示** |
| 下段（内訳） | `SeverityStackChart.vue` | `history` の重大度別積み上げ棒。同じく < 2 は空表示 |

### レイヤ分割（テスト戦略の要）

- **`utils/chart-config.ts`（純粋関数）**: `buildScoreTrendConfig(history, palette)` / `buildSeverityStackConfig(history, palette)`。色はセマンティック名→実値の palette を**引数注入**し、jsdom で CSS 変数解決せずにテストできる形にする。unit 100%。
- **`utils/score-band.ts`（純粋関数）**: Band（`good`/`caution`/`danger`/`crisis`）→ トークンクラス名 + delta 整形。frontend.md「セキュリティスコアの色分け」の実装。未知 Band は `muted` へフォールバック（前方互換）。※状態を持たないため composable でなく utils に置く。
- **`components/dashboard/ChartCanvas.vue`（薄ラッパ）**: props の config で `new Chart()`（onMounted）/ 更新（watch）/ destroy（onUnmounted）のみ。テストは chart.js モック。
- **`composables/useApiError.ts`**: API エラーハンドリング集約（frontend.md 指示）。画面にはエラーバンドで表示。

### エラー / エッジケース

- 不正 site id・不在（400/404）→ 他のエラーと同じエラーバンドで backend のメッセージを表示（`fatal` な client エラーはテスト困難で PoC では過剰なため、専用 404 ページは作らない）。
- ダッシュボード API はスキャン無し・未知サイトでも 200（latest=null / history=[]）→ 空状態表示で吸収。
- API 疎通エラー → `useApiError` 経由の共通エラーバンド。

### テスト（カバレッジ 100% 要件）

- utils / composables: テーブル駆動 unit。境界: history 0/1/2件・delta null/正/負・Band 全種 + 未知値。
- コンポーネント: `mountSuspended` + chart.js モックで空状態/データありの両分岐を網羅。
- Statements / Branches / Functions 100%（除外は PR A のカバレッジ設定に従う）。

## 決定事項の記録

- Chart 統合は **素の chart.js + 自作薄ラッパ**（vue-chartjs は追加しない。YAGNI）。
- ナビ構造は **サイト一覧（index）→ サイト別ダッシュボード（/sites/[id]）**。将来の登録・履歴画面もこの構造に載せる。
- 型生成は swagger.yaml を正とし、生成物をコミットする（PROGRESS.md の決定に準拠）。
