# web (Nuxt 3) スキャフォールド Implementation Plan（PR A）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** web/ を Nuxt 3 でスキャフォールドし、デザイントークン統合・型付き API クライアント・テスト基盤・CI を有効化する（画面はレイアウト骨格のみ）。

**Architecture:** Nuxt 3（SSR・TypeScript strict）+ Tailwind CSS v4。`tokens.css` を Tailwind `@theme` に変換してセマンティックユーティリティ（`bg-canvas` 等）を生成する（値の正は tokens.css のまま）。API 型は `swagger.yaml`（Swagger 2.0）→ `swagger2openapi` → `openapi-typescript` で生成し、`openapi-fetch` で消費する。

**Tech Stack:** Nuxt 3 / pnpm / Tailwind CSS v4（`@tailwindcss/vite`）/ Vitest + @nuxt/test-utils（istanbul coverage）/ ESLint（@nuxt/eslint）/ openapi-typescript + openapi-fetch / @fontsource/inter

## Global Constraints

- ブランチ: `chore/0019-web-scaffold`（作成済み・スペックコミット済み）。コミットは Conventional Commits（日本語・命令形・72字以内）
- 生 hex 値を Vue テンプレート・`<style>` に書かない。`<style>` ブロック・`style="..."` 属性禁止。トークン値の変更は `web/assets/css/tokens.css` のみ
- TypeScript strict。`any` 禁止。`console.log` 禁止。未使用 import 禁止
- カバレッジ: Statements / Branches / Functions **100%**（istanbul。除外: `*.d.ts`・設定ファイル・`assets/`）
- パッケージマネージャは pnpm（Node 22）。作業ディレクトリは `web/`
- 生成物 `web/types/api.d.ts` はコミットする・手編集禁止（sqlc 生成コードと同じ扱い）
- API サーバは `:8080`（`API_ADDR` 既定値）。フロントは `/api` プレフィックスで devProxy 経由アクセス
- コミットフッター: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>` + `Claude-Session: https://claude.ai/code/session_012ZYDjr15MCdtw7sCM262NF`

---

### Task 1: Nuxt 基盤ファイルと依存インストール

**Files:**
- Create: `web/package.json`
- Create: `web/nuxt.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/app.vue`
- Create: `web/pages/index.vue`
- Create: `web/.gitignore`
- Delete: `web/pages/.gitkeep`

**Interfaces:**
- Produces: `useRuntimeConfig().public.apiBase`（string・既定 `/api`）。後続タスクの `useApiClient` が消費

- [ ] **Step 1: package.json を作成**

```json
{
  "name": "goodast-web",
  "private": true,
  "type": "module",
  "packageManager": "pnpm@10.12.1",
  "scripts": {
    "build": "nuxt build",
    "dev": "nuxt dev",
    "preview": "nuxt preview",
    "postinstall": "nuxt prepare",
    "lint": "eslint .",
    "type-check": "nuxt typecheck",
    "test": "vitest",
    "gen:api": "swagger2openapi --patch --yaml --outfile .openapi/openapi3.yaml ../api/internal/docs/swagger.yaml && openapi-typescript .openapi/openapi3.yaml -o types/api.d.ts"
  },
  "dependencies": {
    "@fontsource/inter": "^5.2.0",
    "nuxt": "^3.17.0",
    "openapi-fetch": "^0.14.0",
    "vue": "^3.5.0",
    "vue-router": "^4.5.0"
  },
  "devDependencies": {
    "@nuxt/eslint": "^1.4.0",
    "@nuxt/test-utils": "^3.19.0",
    "@tailwindcss/vite": "^4.1.0",
    "@vitest/coverage-istanbul": "^3.1.0",
    "@vue/test-utils": "^2.4.6",
    "eslint": "^9.28.0",
    "happy-dom": "^17.4.0",
    "openapi-typescript": "^7.8.0",
    "swagger2openapi": "^7.0.8",
    "tailwindcss": "^4.1.0",
    "typescript": "^5.8.0",
    "vitest": "^3.1.0",
    "vue-tsc": "^2.2.0"
  }
}
```

> バージョンは caret 指定。`pnpm install` 時点の最新パッチが lockfile に固定される。chart.js は PR B で追加する（YAGNI）。

- [ ] **Step 2: web/.gitignore を作成**

```
node_modules
.nuxt
.output
dist
coverage
.openapi
*.log
```

- [ ] **Step 3: nuxt.config.ts を作成**

```ts
import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  compatibilityDate: '2026-07-04',
  modules: ['@nuxt/eslint', '@nuxt/test-utils/module'],
  css: ['~/assets/css/main.css'],
  vite: { plugins: [tailwindcss()] },
  typescript: { strict: true },
  runtimeConfig: {
    public: {
      // API のベースパス。開発時は下の devProxy が :8080 の Gin API へ中継する
      apiBase: '/api',
    },
  },
  nitro: {
    devProxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
  app: {
    head: { title: 'goodast', htmlAttrs: { lang: 'ja' } },
  },
})
```

- [ ] **Step 4: tsconfig.json / app.vue / pages/index.vue を作成、pages/.gitkeep を削除**

`web/tsconfig.json`:
```json
{
  "extends": "./.nuxt/tsconfig.json"
}
```

`web/app.vue`:
```vue
<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>
```

`web/pages/index.vue`（仮置き。Task 2 でスタイル、PR B で実装）:
```vue
<template>
  <main>
    <h1>goodast</h1>
  </main>
</template>
```

```bash
rm web/pages/.gitkeep
```

- [ ] **Step 5: 依存をインストールし Nuxt が起動可能なことを確認**

```bash
cd web && pnpm install
```
Expected: 成功し `pnpm-lock.yaml` が生成される（`postinstall` の `nuxt prepare` も成功し `.nuxt/` が生成される）。

> この時点で `~/assets/css/main.css` が未作成のため `pnpm build` は失敗する。ビルド検証は Task 2 の最後で行う。

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/pnpm-lock.yaml web/nuxt.config.ts web/tsconfig.json web/app.vue web/pages/index.vue web/.gitignore
git rm web/pages/.gitkeep
git commit -m "chore(web): Nuxt 3 の基盤ファイルと依存を追加する"
```
（コミットフッターは Global Constraints 参照。以降のコミットも同様）

---

### Task 2: Tailwind v4 + デザイントークン統合 + レイアウト骨格

**Files:**
- Modify: `web/assets/css/tokens.css`（`:root` → `@theme` 化）
- Create: `web/assets/css/main.css`
- Create: `web/layouts/default.vue`
- Modify: `web/pages/index.vue`

**Interfaces:**
- Produces: セマンティックユーティリティ（`bg-canvas` `text-body` `text-on-dark` `border-hairline` `bg-surface-card` `bg-surface-soft` `text-muted` `text-success` `text-warning` `text-m-red` `font-display` `font-body` `text-display-lg` `text-display-sm` `text-title-lg` `text-title-md` `text-label` `text-body-sm` `text-caption` `tracking-label` `tracking-caption` `max-w-page` 等）と `m-stripe` ユーティリティ。PR B の全コンポーネントが消費

> **設計判断**: Tailwind 公式ドキュメントは「`:root` 変数を @theme inline で**別名**マップ」を正としているが、tokens.css は既に `--color-*` 命名で Tailwind の namespace と一致するため、同名マップは循環参照になる。tokens.css 自体を `@theme` に変換すれば、値の定義は tokens.css 一箇所のまま（source of truth 不変）、ユーティリティ生成と `:root` への CSS 変数出力（`var(--color-*)` 参照可）を両立できる。
> typography スケール（`--text-*`）は DESIGN.md の typography 定義の転記であり、新規の値発明ではない。

- [ ] **Step 1: tokens.css を @theme に変換**

全文を以下に置き換える:

```css
/* Design tokens — source of truth: /DESIGN.md */
/* Tailwind v4 の @theme として定義する。ユーティリティ生成（bg-canvas 等）と
   :root への CSS 変数出力（var(--color-*) 参照）を兼ねる。値の変更はこのファイルのみで行う */
@theme {
  /* デフォルトパレット無効化 — セマンティックトークンのみ許可（.claude/rules/frontend.md） */
  --color-*: initial;

  /* Colors — Surface */
  --color-canvas:           #000000;
  --color-surface-soft:     #0d0d0d;
  --color-surface-card:     #1a1a1a;
  --color-surface-elevated: #262626;
  --color-carbon-gray:      #2b2b2b;

  /* Colors — Text */
  --color-on-dark:          #ffffff;
  --color-body-strong:      #e6e6e6;
  --color-body:             #bbbbbb;
  --color-muted:            #7e7e7e;

  /* Colors — Hairline */
  --color-hairline:         #3c3c3c;
  --color-hairline-strong:  #262626;

  /* Colors — Brand / M Tricolor */
  --color-m-blue-light:     #0066b1;
  --color-m-blue-dark:      #1c69d4;
  --color-m-red:            #e22718;
  --color-electric-blue:    #0653b6;

  /* Colors — Semantic */
  --color-warning:          #f4b400;
  --color-success:          #0fa336;

  /* Spacing（named。数値スケール p-4 等と併用可） */
  --spacing-xxs:     4px;
  --spacing-xs:      8px;
  --spacing-sm:      12px;
  --spacing-md:      16px;
  --spacing-lg:      24px;
  --spacing-xl:      40px;
  --spacing-xxl:     64px;
  --spacing-section: 96px;

  /* Border Radius（DESIGN.md: ほぼ 0、円形のみ例外） */
  --radius-*: initial;
  --radius-xs: 2px;
  --radius-sm: 4px;
  --radius-md: 6px;

  /* Typography — DESIGN.md の typography 定義（サイズ/行間）。weight は font-bold(700)/font-light(300) を併用 */
  --text-display-xl: 80px;
  --text-display-xl--line-height: 1;
  --text-display-lg: 56px;
  --text-display-lg--line-height: 1.05;
  --text-display-md: 40px;
  --text-display-md--line-height: 1.1;
  --text-display-sm: 32px;
  --text-display-sm--line-height: 1.15;
  --text-title-lg: 24px;
  --text-title-lg--line-height: 1.3;
  --text-title-md: 20px;
  --text-title-md--line-height: 1.4;
  --text-title-sm: 18px;
  --text-title-sm--line-height: 1.4;
  --text-label: 14px;
  --text-label--line-height: 1.3;
  --text-body-md: 16px;
  --text-body-md--line-height: 1.5;
  --text-body-sm: 14px;
  --text-body-sm--line-height: 1.5;
  --text-caption: 12px;
  --text-caption--line-height: 1.4;

  /* Letter spacing */
  --tracking-label:   1.5px;
  --tracking-caption: 0.5px;

  /* Fonts */
  --font-display: 'BMWTypeNextLatin', 'Inter', sans-serif;
  --font-body:    'BMWTypeNextLatin Light', 'BMWTypeNextLatin', 'Inter', sans-serif;

  /* Container（DESIGN.md: max content width 1440px） */
  --container-page: 1440px;
}

/* Tailwind namespace 外のトークン */
:root {
  /* M Tricolor gradient (brand divider only) */
  --gradient-m-stripe: linear-gradient(
    to right,
    var(--color-m-blue-light),
    var(--color-m-blue-dark),
    var(--color-m-red)
  );
}
```

- [ ] **Step 2: main.css を作成**

`web/assets/css/main.css`:
```css
@import 'tailwindcss';
@import '@fontsource/inter/300.css';
@import '@fontsource/inter/700.css';
@import './tokens.css';

@layer base {
  body {
    background-color: var(--color-canvas);
    color: var(--color-body);
    font-family: var(--font-body);
    font-weight: 300;
  }
}

/* M ストライプ（ブランド区切り線専用。CTA・背景塗り使用禁止） */
@utility m-stripe {
  height: 4px;
  background-image: var(--gradient-m-stripe);
}
```

- [ ] **Step 3: layouts/default.vue を作成**

```vue
<template>
  <div class="min-h-screen bg-canvas font-body text-body">
    <header class="border-b border-hairline">
      <div class="mx-auto flex h-16 w-full max-w-page items-center px-6">
        <NuxtLink
          to="/"
          class="font-display text-title-lg font-bold uppercase tracking-label text-on-dark"
        >
          Goodast
        </NuxtLink>
      </div>
      <div class="m-stripe" />
    </header>
    <main class="mx-auto w-full max-w-page px-6 py-10">
      <slot />
    </main>
  </div>
</template>
```

- [ ] **Step 4: pages/index.vue をトークン準拠のプレースホルダに更新**

```vue
<template>
  <section>
    <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">Sites</h1>
    <p class="mt-6 text-body-sm text-muted">サイト一覧は次のリリースで実装予定です。</p>
  </section>
</template>
```

- [ ] **Step 5: ビルドとユーティリティ生成を確認**

```bash
cd web && pnpm build
grep -l 'bg-canvas' .output/public/_nuxt/*.css
```
Expected: ビルド成功。生成 CSS に `bg-canvas` クラスが含まれる（@theme 変換が機能している証拠）。

- [ ] **Step 6: 開発サーバで目視確認（黒 canvas + ナビ + M ストライプ）**

```bash
cd web && pnpm dev
```
Expected: `http://localhost:3000` で黒背景・白ワードマーク「GOODAST」・4px 三色ストライプが表示される。確認後 Ctrl-C。

- [ ] **Step 7: Commit**

```bash
git add web/assets/css/tokens.css web/assets/css/main.css web/layouts/default.vue web/pages/index.vue
git commit -m "chore(web): tokens.css を Tailwind v4 @theme 化しレイアウト骨格を追加する"
```

---

### Task 3: ESLint 配線

**Files:**
- Create: `web/eslint.config.mjs`

- [ ] **Step 1: eslint.config.mjs を作成**

```js
// @ts-check
import withNuxt from './.nuxt/eslint.config.mjs'

export default withNuxt({
  // openapi-typescript 生成物（手編集禁止）は lint 対象外
  ignores: ['types/api.d.ts'],
})
```

- [ ] **Step 2: lint 実行**

```bash
cd web && pnpm lint
```
Expected: エラー 0。既存ファイルで違反が出た場合はその場で修正する（`pnpm lint --fix` 可）。

- [ ] **Step 3: type-check 実行**

```bash
cd web && pnpm type-check
```
Expected: エラー 0。

- [ ] **Step 4: Commit**

```bash
git add web/eslint.config.mjs
git commit -m "chore(web): ESLint（@nuxt/eslint flat config）を配線する"
```

---

### Task 4: Vitest + カバレッジ設定 + 骨格テスト

**Files:**
- Create: `web/vitest.config.ts`
- Create: `web/tests/layouts/default.spec.ts`
- Create: `web/tests/pages/index.spec.ts`

**Interfaces:**
- Produces: `pnpm test --run --coverage` が 100% しきい値でパスする状態。以降の全タスクはテスト追加時にこのゲートを維持する

- [ ] **Step 1: vitest.config.ts を作成**

```ts
import { defineVitestConfig } from '@nuxt/test-utils/config'

export default defineVitestConfig({
  test: {
    environment: 'nuxt',
    coverage: {
      provider: 'istanbul',
      include: [
        'components/**/*.vue',
        'composables/**/*.ts',
        'layouts/**/*.vue',
        'pages/**/*.vue',
        'utils/**/*.ts',
      ],
      exclude: ['**/*.d.ts'],
      thresholds: { statements: 100, branches: 100, functions: 100 },
    },
  },
})
```

> `app.vue` は DI 配線相当（`<NuxtLayout><NuxtPage/>` のみ）のため backend の `cmd/*/main.go` と同じ扱いで計測対象外。

- [ ] **Step 2: 失敗するテストを書く（layout）**

`web/tests/layouts/default.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import DefaultLayout from '~/layouts/default.vue'

describe('layouts/default', () => {
  it('ワードマーク・M ストライプ・slot 内容を描画する', async () => {
    const wrapper = await mountSuspended(DefaultLayout, {
      slots: { default: () => 'SLOT-CONTENT' },
    })
    expect(wrapper.text()).toContain('Goodast')
    expect(wrapper.text()).toContain('SLOT-CONTENT')
    expect(wrapper.find('.m-stripe').exists()).toBe(true)
    expect(wrapper.find('a[href="/"]').exists()).toBe(true)
  })
})
```

`web/tests/pages/index.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import IndexPage from '~/pages/index.vue'

describe('pages/index', () => {
  it('見出し Sites を描画する', async () => {
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain('Sites')
  })
})
```

- [ ] **Step 3: テスト実行（初回はここで環境不備を洗い出す）**

```bash
cd web && pnpm test --run --coverage
```
Expected: 2 テスト PASS・カバレッジ 100%（対象は layout + index のみ）。環境エラーが出る場合は `@nuxt/test-utils` のバージョンと `environment: 'nuxt'` の設定を確認する。

- [ ] **Step 4: Commit**

```bash
git add web/vitest.config.ts web/tests/
git commit -m "test(web): Vitest とカバレッジ 100% ゲートを配線する"
```

---

### Task 5: API 型生成 + 型付きクライアント composable

**Files:**
- Create: `web/types/api.d.ts`（`pnpm gen:api` で生成・コミット）
- Create: `web/types/goodast.ts`
- Create: `web/composables/useApi.ts`
- Test: `web/tests/composables/useApi.spec.ts`

**Interfaces:**
- Consumes: `api/internal/docs/swagger.yaml`（main にマージ済み・API 契約の正）
- Produces:
  - `useApiClient(): Client<paths>` — openapi-fetch クライアント。`client.GET('/sites')` / `client.GET('/sites/{id}', { params: { path: { id } } })` の形で PR B が消費
  - 型エイリアス: `Site` / `DashboardData` / `HistoryEntry` / `LatestState` / `SeverityCounts` / `Band` / `ApiErrorResponse`（`~/types/goodast`）

- [ ] **Step 1: 型を生成**

```bash
cd web && pnpm gen:api
```
Expected: `.openapi/openapi3.yaml`（gitignore 済み）と `types/api.d.ts` が生成される。`types/api.d.ts` に `'/sites/{id}/dashboard'` のパス定義が含まれることを確認:
```bash
grep -c "sites/{id}/dashboard" types/api.d.ts
```
Expected: 1 以上。

- [ ] **Step 2: 型エイリアスを作成**

`web/types/goodast.ts`:
```ts
// swagger.yaml 由来の生成型（types/api.d.ts）への短縮エイリアス。
// swaggo の定義名は完全修飾名で長いため、ここで一元的に別名を付ける
import type { components } from '~/types/api'

type Schemas = components['schemas']

export type Site = Schemas['internal_handler.siteResponse']
export type ApiErrorResponse = Schemas['internal_handler.ErrorResponse']
export type DashboardData = Schemas['github_com_ymd38_goodast_api_internal_report.DashboardData']
export type HistoryEntry = Schemas['github_com_ymd38_goodast_api_internal_report.HistoryEntry']
export type LatestState = Schemas['github_com_ymd38_goodast_api_internal_report.LatestState']
export type SeverityCounts = Schemas['github_com_ymd38_goodast_api_internal_report.SeverityCounts']
export type Band = Schemas['github_com_ymd38_goodast_api_internal_report.Band']
```

> 生成型のキー名が上記と一致するかは `types/api.d.ts` を開いて確認する（swagger2openapi はスキーマ名を保持するが、`.` がそのまま残る）。異なる場合は実際の生成名に合わせる。

- [ ] **Step 3: 失敗するテストを書く**

`web/tests/composables/useApi.spec.ts`:
```ts
import { afterEach, describe, expect, it, vi } from 'vitest'

describe('useApiClient', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('runtimeConfig の apiBase を前置して fetch する', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const client = useApiClient()
    const { data, error } = await client.GET('/sites')

    expect(error).toBeUndefined()
    expect(data).toEqual([])
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const request = fetchMock.mock.calls[0]![0] as Request
    expect(request.url).toContain('/api/sites')
  })
})
```

- [ ] **Step 4: テストが失敗することを確認**

```bash
cd web && pnpm test --run tests/composables/useApi.spec.ts
```
Expected: FAIL（`useApiClient is not defined`）。

- [ ] **Step 5: composable を実装**

`web/composables/useApi.ts`:
```ts
import createClient from 'openapi-fetch'
import type { paths } from '~/types/api'

// swagger.yaml 生成型で全パス・パラメータ・レスポンスが型付けされたクライアント。
// SSR では相対 baseUrl を解決できないため、データ取得は useAsyncData の
// { server: false } と組み合わせてクライアント側でのみ実行する
export function useApiClient() {
  const { apiBase } = useRuntimeConfig().public
  return createClient<paths>({ baseUrl: apiBase })
}
```

- [ ] **Step 6: テストがパスすることを確認**

```bash
cd web && pnpm test --run --coverage
```
Expected: 全テスト PASS・カバレッジ 100% 維持。

- [ ] **Step 7: lint / type-check**

```bash
cd web && pnpm lint && pnpm type-check
```
Expected: エラー 0（`types/api.d.ts` は lint ignore 済み）。

- [ ] **Step 8: Commit**

```bash
git add web/types/api.d.ts web/types/goodast.ts web/composables/useApi.ts web/tests/composables/useApi.spec.ts
git commit -m "chore(web): swagger.yaml から型付き API クライアントを生成する"
```

---

### Task 6: Makefile + CI 有効化

**Files:**
- Modify: `Makefile`（`setup` / `dev-web` / `test-web` / `test`）
- Modify: `.github/workflows/ci.yml`（frontend ジョブのコメント解除）
- Modify: `.github/workflows/security-scan.yml`（pnpm-audit ジョブのコメント解除）

- [ ] **Step 1: Makefile を更新**

`setup` ターゲット（既存の2行に web を追加）:
```make
.PHONY: setup
setup: ## Go モジュール依存の取得・git hooks 有効化・web の pnpm install
	@for m in $(GO_MODULES); do echo "==> $$m"; (cd $$m && go mod download); done
	@git config core.hooksPath .githooks && echo "==> git hooks 有効化（.githooks）"
	@echo "==> web" && cd web && pnpm install
```

`dev-web` ターゲット（既存の TODO を置換）:
```make
.PHONY: dev-web
dev-web: ## web 開発サーバを起動する（Nuxt・/api は :8080 へ devProxy）
	cd web && pnpm dev
```

`test-web` ターゲットを新設し、`test` に組み込む:
```make
.PHONY: test
test: test-api test-worker test-web ## 全テスト（Go race + web lint/type-check/vitest）

.PHONY: test-web
test-web: ## web テスト（lint + type-check + vitest coverage 100% ゲート）
	cd web && pnpm lint && pnpm type-check && pnpm test --run --coverage
```

- [ ] **Step 2: ci.yml の frontend ジョブをコメント解除**

63〜92行目の `# frontend:` ブロックの行頭 `# ` を除去する。最終形（Test ステップにのみ `--coverage` を追加してカバレッジゲートを CI で強制する）:

```yaml
  frontend:
    name: Frontend (Nuxt)
    runs-on: ubuntu-latest
    if: github.event_name != 'issue_comment'
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 10
      - uses: actions/setup-node@v4
        with:
          node-version: '22'
          cache: pnpm
          cache-dependency-path: web/pnpm-lock.yaml
      - name: Install dependencies
        run: pnpm install --frozen-lockfile
      - name: Dependency audit
        run: pnpm audit --audit-level=high
      - name: Prepare Nuxt
        run: pnpm exec nuxt prepare
      - name: Lint
        run: pnpm lint
      - name: Type check
        run: pnpm type-check
      - name: Test
        run: pnpm test --run --coverage
```

- [ ] **Step 3: security-scan.yml の pnpm-audit ジョブをコメント解除**

`# pnpm-audit:` ブロック（79行目以降）の行頭 `# ` を除去する。最終形:

```yaml
  pnpm-audit:
    name: Frontend Dependency Audit (pnpm)
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 10
      - uses: actions/setup-node@v4
        with:
          node-version: '22'
          cache: pnpm
          cache-dependency-path: web/pnpm-lock.yaml
      - name: Install dependencies
        run: pnpm install --frozen-lockfile
      - name: Frontend dependency audit
        # --prod: devDependencies は本番に含まれないため対象外
        run: pnpm audit --prod --audit-level=high
```

- [ ] **Step 4: ローカルで test-web を検証**

```bash
make test-web
```
Expected: lint / type-check / vitest（カバレッジ 100%）すべてパス。

- [ ] **Step 5: YAML 構文を検証**

```bash
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); yaml.safe_load(open('.github/workflows/security-scan.yml')); print('OK')"
```
Expected: `OK`。

- [ ] **Step 6: Commit**

```bash
git add Makefile .github/workflows/ci.yml .github/workflows/security-scan.yml
git commit -m "chore: web の make ターゲットと CI frontend/pnpm-audit ジョブを有効化する"
```

---

### Task 7: PROGRESS.md 更新 + PR A 作成

**Files:**
- Modify: `PROGRESS.md`（現在地スナップショット・ロードマップの `web (Nuxt) スキャフォールド` チェック・直近アクション）

- [ ] **Step 1: PROGRESS.md を更新**

- 現在地スナップショットに追記: 「**web (Nuxt) スキャフォールド: 完了**（PR A）— Nuxt 3 + Tailwind v4（tokens.css @theme 化）+ Vitest（カバレッジ100%ゲート）+ ESLint + openapi-typescript 型付きクライアント（swagger 2.0→OpenAPI 3 変換経由・生成物コミット）+ CI frontend/pnpm-audit 有効化。**残: ダッシュボード画面（PR B・同設計スペック）**」
- ロードマップ `- [ ] web (Nuxt) スキャフォールド → CI の frontend / pnpm-audit ジョブ有効化` を `[x]` に変更
- 最終更新日を更新

- [ ] **Step 2: セルフレビュー（diff 一読）**

```bash
git diff main...HEAD --stat && git diff main...HEAD
```
frontend.md 違反（生 hex・`<style>`・`any`）がないこと、コミット単位が規約通りであることを確認。

- [ ] **Step 3: Commit + push + PR 作成**

```bash
git add PROGRESS.md
git commit -m "docs: PROGRESS を web スキャフォールド完了に更新する"
git push -u origin chore/0019-web-scaffold
```

PR タイトル: `chore(web): Nuxt 3 スキャフォールドと CI frontend ジョブを有効化する`

PR 本文（`.claude/rules/git.md` のテンプレート準拠）:
```markdown
## 概要
web/ を Nuxt 3 でスキャフォールドし、デザイントークン統合・型付き API クライアント・テスト基盤・CI を有効化する（画面はレイアウト骨格のみ。ダッシュボードは次 PR）。

## 変更内容
- Nuxt 3 + TypeScript strict + pnpm（package.json / nuxt.config.ts / devProxy `/api`→`:8080`）
- tokens.css を Tailwind v4 `@theme` 化（値の正は tokens.css のまま・セマンティックユーティリティ生成・デフォルトパレット無効化）+ DESIGN.md typography スケールの転記
- swagger.yaml → swagger2openapi → openapi-typescript で `types/api.d.ts` 生成（コミット・手編集禁止）+ openapi-fetch の `useApiClient`
- Vitest + @nuxt/test-utils（istanbul カバレッジ 100% しきい値）+ ESLint flat config
- Makefile `dev-web` / `test-web` 実体化、CI の frontend / pnpm-audit ジョブ有効化

## 動作確認
- `make test-web`（lint / type-check / vitest カバレッジ 100%）パス
- `pnpm build` 成功・生成 CSS に `bg-canvas` ユーティリティを確認
- `pnpm dev` で黒 canvas + GOODAST ナビ + M ストライプを目視確認

## 関連 Issue
（設計スペック: docs/superpowers/specs/2026-07-04-web-scaffold-dashboard-design.md）
```

```bash
gh pr create --title "chore(web): Nuxt 3 スキャフォールドと CI frontend ジョブを有効化する" --body "<上記本文 + 🤖 Generated with [Claude Code](https://claude.com/claude-code) フッター>"
```
