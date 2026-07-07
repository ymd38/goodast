# UI一気通貫フロー（v1・コア4画面）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **This is a frontend (web/) plan — run it in a dedicated frontend session (CLAUDE.md front/back separation).** For component visual polish, follow `DESIGN.md` + `.claude/rules/frontend.md`; the markup in this plan is a working baseline built from the existing `components/dashboard/*` vocabulary.

**Goal:** ブラウザだけで「サイト登録 → 所有確認 → スキャン設定（プリセット）→ 進捗 → 結果レポート」を一気通貫で体験できる web UI を実装する。

**Architecture:** Nuxt 3 + `<script setup>`。取得は既存の型付き `useApiClient()`（openapi-fetch）＋ `useAsyncData({server:false})`、ミューテーションはクライアント直呼び＋ローカル状態。純粋ロジック（プリセット定義・重大度→色・ポーリング状態機械）は util/composable に切り出し Vitest 100%。色は tokens.css セマンティッククラス経由。

**Tech Stack:** Nuxt 3 / TypeScript strict / Tailwind (tokens.css) / Vitest + @vue/test-utils + @nuxt/test-utils / openapi-fetch

## Global Constraints

- `<script setup>` + Composition API 必須。`any`・`console.log`（本番）・直接 DOM 操作・未使用 import 禁止。
- 色・サイズ・余白は `web/assets/css/tokens.css` の CSS 変数を参照するセマンティッククラス経由（`on-dark`/`surface-card`/`surface-soft`/`hairline`/`muted`/`success`/`warning`/`m-red` 等）。**生 hex 禁止・`<style>` ブロック禁止・`style=""` インライン禁止**（Tailwind で表現不可時のみ例外＋コメント）。
- 重大度色: Critical/High=`text-m-red`、Medium=`text-warning`、Low/Info=`text-muted`（スコアバンドは既存 `utils/score-band.ts` に準拠）。重大度→クラスは純粋 util に集約。
- Mobile-first・レスポンシブ。固定 px 幅（`w-[375px]` 等）禁止。`w-full`/`max-w-*`/`grid-cols-*`。
- 認証情報は扱わない（v1 対象外）。ただし本フローに認証入力は無いので該当なし。
- テスト: Vitest（Istanbul）で Statements/Branches/Functions **100%**。純粋ロジックは util/composable へ切り出して網羅。コンポーネントは `mount`（`@vue/test-utils`）で描画/分岐を網羅。API はモック。
- 検証: `web/` で `pnpm test --coverage`（= `make test-web`）と `pnpm lint`。Node は nvm の Node 22 を使う（`/usr/local/bin/node` v0.10 が覆う場合は nvm bin を PATH 前置）。
- API 契約は `api/internal/docs/swagger.yaml` 由来の生成型 `web/types/api.d.ts`。型エイリアスは `web/types/goodast.ts` に追加する。
- 既存パターン参照: `web/components/dashboard/ScoreCard.vue`（props+computed+testid）、`web/tests/components/dashboard/ScoreCard.spec.ts`（mount+text/classes）、`web/utils/score-band.ts`（semantic→class）、`web/composables/useApi.ts`（`useApiClient`）、`web/composables/useApiError.ts`（`toApiErrorMessage`）。
- プリセット値（backend `jobs.Preset` と一致必須）: `light`（軽量）/ `standard`（標準・既定）/ `deep`（詳細）。
- 各ページのデータ取得は `useAsyncData(key, fn, { server: false, default: ... })`（SSR で相対 apiBase を解決できないため client 側のみ）。

---

## 型エイリアスの追加（PR-A の Task 0 として先に行う）

### Task 0: types/goodast.ts に scan 系エイリアスを追加

**Files:**
- Modify: `web/types/goodast.ts`

**Interfaces:**
- Produces: `ScanState`, `ScanSummary`, `Finding`, `SiteResponse`（型エイリアス）

- [ ] **Step 1: 生成型のスキーマ名を確認**

Run: `grep -oE "github_com_ymd38_goodast_api_internal_report\.(ScanState|Finding|ScanSummary)|internal_handler\.siteResponse" web/types/api.d.ts | sort -u`
Expected: `internal_handler.siteResponse` と `...report.ScanState` / `.Finding` / `.ScanSummary` が存在する（無ければ `make swagger` 再生成が必要）。

- [ ] **Step 2: エイリアス追加**

`web/types/goodast.ts` の末尾（既存エイリアス群の下）に追記:
```ts
export type SiteResponse = Schemas['internal_handler.siteResponse']
export type ScanState = Schemas['github_com_ymd38_goodast_api_internal_report.ScanState']
export type ScanSummary = Schemas['github_com_ymd38_goodast_api_internal_report.ScanSummary']
export type Finding = Schemas['github_com_ymd38_goodast_api_internal_report.Finding']
```
> `Site`（既存）は `siteResponse` と同一スキーマ。登録レスポンスは `SiteResponse` として明示利用してよい（同義）。`verification`/`verify_token` は生成型に含まれる（optional）。

- [ ] **Step 3: 型チェック**

Run: `cd web && pnpm exec nuxt typecheck` （または `pnpm exec vue-tsc --noEmit`。既存の typecheck スクリプトがあればそれ）
Expected: エラーなし。

- [ ] **Step 4: Commit**

```bash
git add web/types/goodast.ts
git commit -m "feat(web): scan 系レスポンスの型エイリアスを追加する"
```
（コミット本文末尾に `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>` と `Claude-Session: https://claude.ai/code/session_012ZYDjr15MCdtw7sCM262NF` を付与。以降の全コミットで同様。）

---

## PR-A: サイト登録 + 所有確認

### Task A1: OwnershipGuide コンポーネント（所有確認ガイドの図解）

**Files:**
- Create: `web/components/site/OwnershipGuide.vue`
- Test: `web/tests/components/site/OwnershipGuide.spec.ts`

**Interfaces:**
- Consumes: `SiteResponse['verification']`（`{ method, file_path?, file_content?, dns_record? }`）
- Produces: `<OwnershipGuide :verification="..." />`（method で file/DNS 手順を切替表示）

- [ ] **Step 1: Write the failing test**

`web/tests/components/site/OwnershipGuide.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import OwnershipGuide from '~/components/site/OwnershipGuide.vue'

describe('OwnershipGuide', () => {
  it('file 方式はファイルパスと内容の手順を表示する', () => {
    const w = mount(OwnershipGuide, {
      props: { verification: { method: 'file', file_path: '/.well-known/goodast.txt', file_content: 'token-abc' } },
    })
    expect(w.text()).toContain('/.well-known/goodast.txt')
    expect(w.text()).toContain('token-abc')
    expect(w.find('[data-testid="guide-file"]').exists()).toBe(true)
    expect(w.find('[data-testid="guide-dns"]').exists()).toBe(false)
  })

  it('dns 方式は TXT レコードの手順を表示する', () => {
    const w = mount(OwnershipGuide, {
      props: { verification: { method: 'dns', dns_record: 'goodast-verify=token-abc' } },
    })
    expect(w.text()).toContain('goodast-verify=token-abc')
    expect(w.find('[data-testid="guide-dns"]').exists()).toBe(true)
    expect(w.find('[data-testid="guide-file"]').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/components/site/OwnershipGuide.spec.ts`
Expected: FAIL（コンポーネント未作成）

- [ ] **Step 3: Write the component**

`web/components/site/OwnershipGuide.vue`:
```vue
<script setup lang="ts">
import type { SiteResponse } from '~/types/goodast'

defineProps<{ verification: NonNullable<SiteResponse['verification']> }>()
</script>

<template>
  <section class="border border-hairline bg-surface-card p-6">
    <h3 class="font-display text-label font-bold uppercase tracking-label text-muted">
      ドメイン所有確認
    </h3>
    <ol v-if="verification.method === 'file'" data-testid="guide-file" class="mt-4 space-y-3 text-body-sm">
      <li>1. 次のパスにファイルを作成します：<code class="text-on-dark">{{ verification.file_path }}</code></li>
      <li>2. ファイルに次の内容を書き込みます：<code class="block bg-surface-soft p-2 text-on-dark">{{ verification.file_content }}</code></li>
      <li>3. 公開できたら「確認する」を押してください。</li>
    </ol>
    <ol v-else data-testid="guide-dns" class="mt-4 space-y-3 text-body-sm">
      <li>1. DNS に次の TXT レコードを追加します：<code class="block bg-surface-soft p-2 text-on-dark">{{ verification.dns_record }}</code></li>
      <li>2. 反映されたら「確認する」を押してください。</li>
    </ol>
  </section>
</template>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/components/site/OwnershipGuide.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/components/site/OwnershipGuide.vue web/tests/components/site/OwnershipGuide.spec.ts
git commit -m "feat(web): 所有確認ガイド（file/DNS 手順図解）コンポーネントを追加する"
```

---

### Task A2: SiteRegisterForm コンポーネント

**Files:**
- Create: `web/components/site/SiteRegisterForm.vue`
- Test: `web/tests/components/site/SiteRegisterForm.spec.ts`

**Interfaces:**
- Produces: `<SiteRegisterForm :submitting="bool" :error="string|null" @submit="(payload:{name,base_url,verify_method}) => void" />`
- ページ側がフォーム値を受け取り API を呼ぶ（フォームは表示と emit に専念・純粋に保つ）。

- [ ] **Step 1: Write the failing test**

`web/tests/components/site/SiteRegisterForm.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SiteRegisterForm from '~/components/site/SiteRegisterForm.vue'

describe('SiteRegisterForm', () => {
  it('入力して submit すると payload を emit する', async () => {
    const w = mount(SiteRegisterForm, { props: { submitting: false, error: null } })
    await w.find('[data-testid="field-name"]').setValue('My Site')
    await w.find('[data-testid="field-base-url"]').setValue('http://localhost:3001')
    await w.find('form').trigger('submit.prevent')
    const emitted = w.emitted('submit')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({ name: 'My Site', base_url: 'http://localhost:3001', verify_method: 'file' })
  })

  it('submitting 中は送信ボタンを無効化する', () => {
    const w = mount(SiteRegisterForm, { props: { submitting: true, error: null } })
    expect(w.find('[data-testid="submit"]').attributes('disabled')).toBeDefined()
  })

  it('error があれば表示する', () => {
    const w = mount(SiteRegisterForm, { props: { submitting: false, error: '登録に失敗しました' } })
    expect(w.text()).toContain('登録に失敗しました')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/components/site/SiteRegisterForm.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the component**

`web/components/site/SiteRegisterForm.vue`:
```vue
<script setup lang="ts">
defineProps<{ submitting: boolean; error: string | null }>()
const emit = defineEmits<{ submit: [{ name: string; base_url: string; verify_method: string }] }>()

const name = ref('')
const baseUrl = ref('')
const verifyMethod = ref('file')

function onSubmit() {
  emit('submit', { name: name.value, base_url: baseUrl.value, verify_method: verifyMethod.value })
}
</script>

<template>
  <form class="space-y-6" @submit.prevent="onSubmit">
    <div>
      <label class="block text-label font-bold uppercase tracking-label text-muted" for="name">サイト名</label>
      <input id="name" v-model="name" data-testid="field-name" required
        class="mt-2 w-full border border-hairline bg-surface-card p-3 text-body text-on-dark" />
    </div>
    <div>
      <label class="block text-label font-bold uppercase tracking-label text-muted" for="base-url">ベース URL</label>
      <input id="base-url" v-model="baseUrl" data-testid="field-base-url" type="url" required
        class="mt-2 w-full border border-hairline bg-surface-card p-3 text-body text-on-dark" />
    </div>
    <div>
      <label class="block text-label font-bold uppercase tracking-label text-muted" for="method">所有確認方式</label>
      <select id="method" v-model="verifyMethod" data-testid="field-method"
        class="mt-2 w-full border border-hairline bg-surface-card p-3 text-body text-on-dark">
        <option value="file">ファイル設置</option>
        <option value="dns">DNS TXT</option>
      </select>
    </div>
    <p v-if="error" class="border border-m-red p-4 text-body-sm text-m-red">{{ error }}</p>
    <button type="submit" data-testid="submit" :disabled="submitting"
      class="bg-on-dark px-6 py-3 font-display text-label font-bold uppercase tracking-label text-canvas disabled:opacity-50">
      {{ submitting ? '登録中…' : 'サイトを登録' }}
    </button>
  </form>
</template>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/components/site/SiteRegisterForm.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/components/site/SiteRegisterForm.vue web/tests/components/site/SiteRegisterForm.spec.ts
git commit -m "feat(web): サイト登録フォームコンポーネントを追加する"
```

---

### Task A3: /sites/new ページ（登録→確認オーケストレーション）

**Files:**
- Create: `web/pages/sites/new.vue`
- Test: `web/tests/pages/sites-new.spec.ts`

**Interfaces:**
- Consumes: `SiteRegisterForm`（A2）, `OwnershipGuide`（A1）, `useApiClient`, `toApiErrorMessage`
- 挙動: submit→`POST /sites`→ verified なら成功メッセージ＋サイトへ導線 / 未確認なら `OwnershipGuide`＋「確認する」→`POST /sites/:id/verify`（422 メッセージ）。

- [ ] **Step 1: Write the failing test**

`web/tests/pages/sites-new.spec.ts`（`useApiClient` をモック。既存の `tests/pages/*.spec.ts` のモック手法に合わせる）:
```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'

const post = vi.fn()
vi.mock('~/composables/useApi', () => ({ useApiClient: () => ({ POST: post }) }))

import SitesNew from '~/pages/sites/new.vue'

describe('pages/sites/new', () => {
  beforeEach(() => post.mockReset())

  it('localhost 登録は即 verified で成功導線を表示する', async () => {
    post.mockResolvedValueOnce({ data: { id: 'site-1', name: 'X', base_url: 'http://localhost:3001', ownership_verified: true }, error: undefined })
    const w = await mountSuspended(SitesNew)
    await w.find('[data-testid="field-name"]').setValue('X')
    await w.find('[data-testid="field-base-url"]').setValue('http://localhost:3001')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r))
    expect(post).toHaveBeenCalledWith('/sites', expect.objectContaining({ body: expect.objectContaining({ name: 'X' }) }))
    expect(w.text()).toContain('登録が完了しました')
    expect(w.find('[data-testid="go-site"]').exists()).toBe(true)
  })

  it('非ローカルは所有確認ガイドを表示し、確認成功で verified になる', async () => {
    post
      .mockResolvedValueOnce({ data: { id: 'site-2', name: 'Y', base_url: 'https://example.com', ownership_verified: false, verification: { method: 'file', file_path: '/x', file_content: 'tok' } }, error: undefined })
      .mockResolvedValueOnce({ data: { id: 'site-2', ownership_verified: true }, error: undefined })
    const w = await mountSuspended(SitesNew)
    await w.find('[data-testid="field-name"]').setValue('Y')
    await w.find('[data-testid="field-base-url"]').setValue('https://example.com')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r))
    expect(w.find('[data-testid="guide-file"]').exists()).toBe(true)
    await w.find('[data-testid="verify"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('所有確認が完了しました')
  })

  it('登録エラーはメッセージを表示する', async () => {
    post.mockResolvedValueOnce({ data: undefined, error: { error: 'サイト名は既に使われています' } })
    const w = await mountSuspended(SitesNew)
    await w.find('[data-testid="field-name"]').setValue('dup')
    await w.find('[data-testid="field-base-url"]').setValue('http://localhost:3001')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('サイト名は既に使われています')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/pages/sites-new.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the page**

`web/pages/sites/new.vue`:
```vue
<script setup lang="ts">
import type { SiteResponse } from '~/types/goodast'

const client = useApiClient()
const submitting = ref(false)
const registerError = ref<string | null>(null)
const site = ref<SiteResponse | null>(null)
const verifying = ref(false)
const verifyError = ref<string | null>(null)
const verifiedNow = ref(false)

async function register(payload: { name: string; base_url: string; verify_method: string }) {
  submitting.value = true
  registerError.value = null
  const { data, error } = await client.POST('/sites', { body: payload })
  submitting.value = false
  if (error || !data) {
    registerError.value = toApiErrorMessage(error)
    return
  }
  site.value = data
}

async function verify() {
  if (!site.value) return
  verifying.value = true
  verifyError.value = null
  const { data, error } = await client.POST('/sites/{id}/verify', { params: { path: { id: site.value.id } } })
  verifying.value = false
  if (error || !data) {
    verifyError.value = toApiErrorMessage(error) || 'まだ設置が確認できません。反映を待って再度お試しください。'
    return
  }
  site.value = data
  verifiedNow.value = true
}

const needsGuide = computed(() => site.value && !site.value.ownership_verified && site.value.verification)
</script>

<template>
  <section class="mx-auto max-w-xl">
    <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">サイトを登録</h1>

    <SiteRegisterForm v-if="!site" class="mt-8" :submitting="submitting" :error="registerError" @submit="register" />

    <div v-else class="mt-8 space-y-6">
      <p v-if="site.ownership_verified" class="border border-success p-4 text-body-sm text-success">
        {{ verifiedNow ? '所有確認が完了しました。' : '登録が完了しました（このサイトは所有確認が不要です）。' }}
      </p>
      <template v-if="needsGuide">
        <OwnershipGuide :verification="site.verification!" />
        <p v-if="verifyError" class="border border-m-red p-4 text-body-sm text-m-red">{{ verifyError }}</p>
        <button data-testid="verify" :disabled="verifying" @click="verify"
          class="bg-on-dark px-6 py-3 font-display text-label font-bold uppercase tracking-label text-canvas disabled:opacity-50">
          {{ verifying ? '確認中…' : '確認する' }}
        </button>
      </template>
      <NuxtLink v-if="site.ownership_verified" :to="`/sites/${site.id}`" data-testid="go-site"
        class="inline-block border border-on-dark px-6 py-3 font-display text-label font-bold uppercase tracking-label text-on-dark">
        サイトを開く
      </NuxtLink>
    </div>
  </section>
</template>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/pages/sites-new.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/pages/sites/new.vue web/tests/pages/sites-new.spec.ts
git commit -m "feat(web): サイト登録ページ（登録→所有確認）を追加する"
```

---

### Task A4: トップページに登録導線を追加

**Files:**
- Modify: `web/pages/index.vue`
- Modify: `web/tests/pages/index.spec.ts`

- [ ] **Step 1: Update the test**

`web/tests/pages/index.spec.ts` に「登録リンクが表示される」ケースを追加（既存テストの mock を流用）:
```ts
it('サイト登録への導線を表示する', async () => {
  // 既存の sites 取得 mock（空でも可）でマウントし、リンクを検証
  const w = /* 既存のマウント手順に合わせる */ await mountIndex([])
  const link = w.find('[data-testid="register-link"]')
  expect(link.exists()).toBe(true)
  expect(link.attributes('href')).toBe('/sites/new')
})
```
（`mountIndex` は既存 spec のヘルパ名に合わせる。無ければ既存の mount 手順をそのまま使う。）

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/pages/index.spec.ts`
Expected: FAIL（リンク未実装）

- [ ] **Step 3: Add the link**

`web/pages/index.vue` の `<h1>Sites</h1>` の並びに登録導線を追加し、未登録時の「API 経由で登録」の注記を差し替える:
```vue
    <div class="flex items-center justify-between">
      <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">Sites</h1>
      <NuxtLink to="/sites/new" data-testid="register-link"
        class="border border-on-dark px-4 py-2 font-display text-label font-bold uppercase tracking-label text-on-dark">
        サイトを登録
      </NuxtLink>
    </div>
```
未登録メッセージを「サイトが未登録です。右上の『サイトを登録』から追加できます。」に変更する（`data-testid="register-link"` を保持）。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/pages/index.spec.ts`
Expected: PASS

- [ ] **Step 5: Full suite + coverage for PR-A**

Run: `cd web && pnpm test --coverage`
Expected: 全 PASS・Statements/Branches/Functions 100%。落ちる場合は未網羅分岐にテストを追加。

- [ ] **Step 6: Commit**

```bash
git add web/pages/index.vue web/tests/pages/index.spec.ts
git commit -m "feat(web): トップにサイト登録導線を追加する"
```

> **PR-A ここまで**。`make test-web` 緑を確認し PR を作成（base main）。

---

## PR-B: スキャン設定ウィザード + スキャン開始

### Task B1: scan-preset util（プリセット定義）

**Files:**
- Create: `web/utils/scan-preset.ts`
- Test: `web/tests/utils/scan-preset.spec.ts`

**Interfaces:**
- Produces: `SCAN_PRESETS: ScanPresetOption[]`（`{ value: 'light'|'standard'|'deep'; label; description; estimate }`）, `DEFAULT_PRESET = 'standard'`

- [ ] **Step 1: Write the failing test**

`web/tests/utils/scan-preset.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { SCAN_PRESETS, DEFAULT_PRESET } from '~/utils/scan-preset'

describe('scan-preset', () => {
  it('3 プリセットを backend の値で定義する', () => {
    expect(SCAN_PRESETS.map((p) => p.value)).toEqual(['light', 'standard', 'deep'])
    for (const p of SCAN_PRESETS) {
      expect(p.label).toBeTruthy()
      expect(p.description).toBeTruthy()
      expect(p.estimate).toBeTruthy()
    }
  })
  it('既定は standard', () => {
    expect(DEFAULT_PRESET).toBe('standard')
    expect(SCAN_PRESETS.some((p) => p.value === DEFAULT_PRESET)).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/utils/scan-preset.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the util**

`web/utils/scan-preset.ts`:
```ts
export type ScanPresetValue = 'light' | 'standard' | 'deep'

export interface ScanPresetOption {
  value: ScanPresetValue
  label: string
  description: string
  estimate: string
}

// backend jobs.Preset と一致（軽量/標準/詳細）。所要目安は engine.PlanFor のタイムアウト上限に対応。
export const SCAN_PRESETS: ScanPresetOption[] = [
  { value: 'light', label: '軽量', description: '基本的な設定ミス・技術検出のみ。素早く確認したいとき。', estimate: '目安 5 分以内' },
  { value: 'standard', label: '標準', description: '公開パネル・既定ログイン・CVE を含むバランス設定（推奨）。', estimate: '目安 15 分以内' },
  { value: 'deep', label: '詳細', description: 'XSS/SQLi/SSRF 等まで広く検査。時間をかけて網羅したいとき。', estimate: '目安 30 分以内' },
]

export const DEFAULT_PRESET: ScanPresetValue = 'standard'
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/utils/scan-preset.spec.ts`
Expected: PASS（100% カバレッジ）

- [ ] **Step 5: Commit**

```bash
git add web/utils/scan-preset.ts web/tests/utils/scan-preset.spec.ts
git commit -m "feat(web): スキャンプリセット定義 util を追加する"
```

---

### Task B2: ScanPresetPicker コンポーネント

**Files:**
- Create: `web/components/scan/ScanPresetPicker.vue`
- Test: `web/tests/components/scan/ScanPresetPicker.spec.ts`

**Interfaces:**
- Consumes: `SCAN_PRESETS`（B1）
- Produces: `<ScanPresetPicker v-model="preset" />`（`modelValue: ScanPresetValue`・カード選択で `update:modelValue`）

- [ ] **Step 1: Write the failing test**

`web/tests/components/scan/ScanPresetPicker.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ScanPresetPicker from '~/components/scan/ScanPresetPicker.vue'

describe('ScanPresetPicker', () => {
  it('3 プリセットを描画し、選択中を強調する', () => {
    const w = mount(ScanPresetPicker, { props: { modelValue: 'standard' } })
    const cards = w.findAll('[data-testid="preset-card"]')
    expect(cards).toHaveLength(3)
    expect(w.find('[data-testid="preset-standard"]').classes()).toContain('border-on-dark')
  })
  it('カードクリックで update:modelValue を emit する', async () => {
    const w = mount(ScanPresetPicker, { props: { modelValue: 'standard' } })
    await w.find('[data-testid="preset-deep"]').trigger('click')
    expect(w.emitted('update:modelValue')![0][0]).toBe('deep')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/components/scan/ScanPresetPicker.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the component**

`web/components/scan/ScanPresetPicker.vue`:
```vue
<script setup lang="ts">
import { SCAN_PRESETS, type ScanPresetValue } from '~/utils/scan-preset'

defineProps<{ modelValue: ScanPresetValue }>()
const emit = defineEmits<{ 'update:modelValue': [ScanPresetValue] }>()
</script>

<template>
  <div class="grid gap-4 md:grid-cols-3">
    <button
      v-for="p in SCAN_PRESETS"
      :key="p.value"
      type="button"
      data-testid="preset-card"
      :data-testid-value="`preset-${p.value}`"
      :class="[
        'border p-6 text-left transition-colors',
        modelValue === p.value ? 'border-on-dark bg-surface-card' : 'border-hairline bg-surface-soft',
      ]"
      @click="emit('update:modelValue', p.value)"
    >
      <p class="font-display text-title-md font-bold text-on-dark">{{ p.label }}</p>
      <p class="mt-2 text-body-sm">{{ p.description }}</p>
      <p class="mt-3 text-caption uppercase tracking-caption text-muted">{{ p.estimate }}</p>
    </button>
  </div>
</template>
```
> テストの `[data-testid="preset-standard"]` 参照に合わせるため、各カードに `:data-testid="`preset-${p.value}`"` を付ける（上記 `data-testid` を `:data-testid="`preset-${p.value}`"` に置換し、一覧選択用の共通フックは別途 `data-preset-card` 等で持つ）。実装時に **テストのセレクタと属性を一致**させること（`findAll('[data-testid="preset-card"]')` と `find('[data-testid="preset-standard"]')` の両方が引けるよう、共通フックは `class` か `data-preset` 属性で持ち、`data-testid` は値別にする）。

- [ ] **Step 4: Align selectors, run test to verify it passes**

テストとコンポーネントのセレクタを一致させる（`data-testid="preset-card"` を全カード共通に、値別は `data-testid="preset-<value>"` にできないため、テスト側を「共通は `.preset-card` クラス、値別は `data-testid`」に統一するか、コンポーネント側を「共通 `data-testid="preset-card"` + `:data-value`」に統一する。**どちらかに決めて両ファイルを揃える**）。
Run: `cd web && pnpm exec vitest run tests/components/scan/ScanPresetPicker.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/components/scan/ScanPresetPicker.vue web/tests/components/scan/ScanPresetPicker.spec.ts
git commit -m "feat(web): スキャンプリセット選択コンポーネントを追加する"
```

---

### Task B3: /sites/[id]/scan ページ（ウィザード）

**Files:**
- Create: `web/pages/sites/[id]/scan.vue`
- Test: `web/tests/pages/sites-id-scan.spec.ts`

**Interfaces:**
- Consumes: `ScanPresetPicker`（B2）, `DEFAULT_PRESET`（B1）, `useApiClient`, `navigateTo`
- 挙動: プリセット選択＋危険パス除外の情報表示。「スキャン開始」→`POST /scans {site_id, preset}`→ 202 の `scan_id` で `/scans/[id]` へ `navigateTo`。

- [ ] **Step 1: Write the failing test**

`web/tests/pages/sites-id-scan.spec.ts`:
```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'

const post = vi.fn()
const navigate = vi.fn()
vi.mock('~/composables/useApi', () => ({ useApiClient: () => ({ POST: post }) }))
vi.mock('#app', async (orig) => ({ ...(await orig() as object), navigateTo: navigate, useRoute: () => ({ params: { id: 'site-1' } }) }))

import ScanWizard from '~/pages/sites/[id]/scan.vue'

describe('pages/sites/[id]/scan', () => {
  beforeEach(() => { post.mockReset(); navigate.mockReset() })

  it('危険パス除外の情報表示とプリセットを描画する', async () => {
    const w = await mountSuspended(ScanWizard)
    expect(w.text()).toContain('自動で除外')
    expect(w.findAll('[data-testid="preset-card"]').length).toBe(3)
  })

  it('スキャン開始で POST /scans し結果ページへ遷移する', async () => {
    post.mockResolvedValueOnce({ data: { scan_id: 'scan-9', status: 'queued', preset: 'standard' }, error: undefined })
    const w = await mountSuspended(ScanWizard)
    await w.find('[data-testid="start-scan"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(post).toHaveBeenCalledWith('/scans', { body: { site_id: 'site-1', preset: 'standard' } })
    expect(navigate).toHaveBeenCalledWith('/scans/scan-9')
  })

  it('開始エラーはメッセージを表示する', async () => {
    post.mockResolvedValueOnce({ data: undefined, error: { error: '所有確認が必要です' } })
    const w = await mountSuspended(ScanWizard)
    await w.find('[data-testid="start-scan"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('所有確認が必要です')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/pages/sites-id-scan.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the page**

`web/pages/sites/[id]/scan.vue`:
```vue
<script setup lang="ts">
import { DEFAULT_PRESET, type ScanPresetValue } from '~/utils/scan-preset'

const route = useRoute()
const siteId = route.params.id as string
const client = useApiClient()

const preset = ref<ScanPresetValue>(DEFAULT_PRESET)
const starting = ref(false)
const startError = ref<string | null>(null)

async function start() {
  starting.value = true
  startError.value = null
  const { data, error } = await client.POST('/scans', { body: { site_id: siteId, preset: preset.value } })
  starting.value = false
  if (error || !data) {
    startError.value = toApiErrorMessage(error)
    return
  }
  await navigateTo(`/scans/${data.scan_id}`)
}
</script>

<template>
  <section class="mx-auto max-w-3xl">
    <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">スキャン設定</h1>

    <h2 class="mt-8 text-label font-bold uppercase tracking-label text-muted">プリセット</h2>
    <ScanPresetPicker v-model="preset" class="mt-4" />

    <div class="mt-8 border border-hairline bg-surface-soft p-6">
      <h2 class="text-label font-bold uppercase tracking-label text-muted">安全設定</h2>
      <p class="mt-2 text-body-sm">
        <code>logout</code> / <code>signout</code> / <code>delete</code> などの危険パスは
        <span class="text-success">自動で除外</span>されます。破壊的なテンプレートも既定で無効です。
      </p>
    </div>

    <p v-if="startError" class="mt-6 border border-m-red p-4 text-body-sm text-m-red">{{ startError }}</p>
    <button data-testid="start-scan" :disabled="starting" class="mt-8 bg-on-dark px-6 py-3 font-display text-label font-bold uppercase tracking-label text-canvas disabled:opacity-50" @click="start">
      {{ starting ? '開始中…' : 'スキャンを開始' }}
    </button>
  </section>
</template>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/pages/sites-id-scan.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/pages/sites/\[id\]/scan.vue web/tests/pages/sites-id-scan.spec.ts
git commit -m "feat(web): スキャン設定ウィザードページを追加する"
```

---

### Task B4: /sites/[id] に「新規スキャン」導線・所有未確認バナー

**Files:**
- Modify: `web/pages/sites/[id].vue`
- Modify: `web/tests/pages/sites-id.spec.ts`

- [ ] **Step 1: Update the test**

`web/tests/pages/sites-id.spec.ts` に2ケース追加（既存の site/dashboard 取得 mock を流用）:
```ts
it('verified サイトはスキャン開始導線を表示する', async () => {
  const w = /* 既存マウント、site.ownership_verified=true */ await mountSiteId({ ownership_verified: true })
  const link = w.find('[data-testid="new-scan"]')
  expect(link.exists()).toBe(true)
  expect(link.attributes('href')).toBe('/sites/site-1/scan')
})
it('未確認サイトはバナーを表示しスキャン導線を出さない', async () => {
  const w = await mountSiteId({ ownership_verified: false })
  expect(w.text()).toContain('所有確認が未完了')
  expect(w.find('[data-testid="new-scan"]').exists()).toBe(false)
})
```
（`mountSiteId` は既存 spec のヘルパ/手順に合わせる。site の `ownership_verified` を差し替えられる形にする。）

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/pages/sites-id.spec.ts`
Expected: FAIL

- [ ] **Step 3: Add link + banner**

`web/pages/sites/[id].vue` のヘッダ付近に、`site.ownership_verified` に応じて分岐を追加:
```vue
    <div class="flex items-center justify-between">
      <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">{{ site?.name }}</h1>
      <NuxtLink v-if="site?.ownership_verified" :to="`/sites/${siteId}/scan`" data-testid="new-scan"
        class="border border-on-dark px-4 py-2 font-display text-label font-bold uppercase tracking-label text-on-dark">
        新規スキャン
      </NuxtLink>
    </div>
    <p v-if="site && !site.ownership_verified" class="mt-4 border border-warning p-4 text-body-sm text-warning">
      所有確認が未完了のためスキャンを実行できません。登録画面から確認を完了してください。
    </p>
```
（既存の変数名 `site` / `siteId` に合わせる。無ければ既存の取得結果に合わせて調整。）

- [ ] **Step 4: Run test + full coverage for PR-B**

Run: `cd web && pnpm exec vitest run tests/pages/sites-id.spec.ts && pnpm test --coverage`
Expected: 全 PASS・100%。

- [ ] **Step 5: Commit**

```bash
git add web/pages/sites/\[id\].vue web/tests/pages/sites-id.spec.ts
git commit -m "feat(web): サイト詳細に新規スキャン導線と所有未確認バナーを追加する"
```

> **PR-B ここまで**。`make test-web` 緑を確認し PR を作成。

---

## PR-C: 進捗 + 結果レポート

### Task C1: severity util（重大度→色/順序）

**Files:**
- Create: `web/utils/severity.ts`
- Test: `web/tests/utils/severity.spec.ts`

**Interfaces:**
- Produces: `severityTextClass(sev: string): string`（Critical/High→`text-m-red`、Medium→`text-warning`、Low/Info→`text-muted`、未知→`text-muted`）, `SEVERITY_ORDER: string[]`（Critical→Info）, `sortFindingsBySeverity(findings)`

- [ ] **Step 1: Write the failing test**

`web/tests/utils/severity.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { severityTextClass, SEVERITY_ORDER, sortFindingsBySeverity } from '~/utils/severity'

describe('severity', () => {
  it('重大度をトークンクラスにマップする', () => {
    expect(severityTextClass('Critical')).toBe('text-m-red')
    expect(severityTextClass('High')).toBe('text-m-red')
    expect(severityTextClass('Medium')).toBe('text-warning')
    expect(severityTextClass('Low')).toBe('text-muted')
    expect(severityTextClass('Info')).toBe('text-muted')
    expect(severityTextClass('???')).toBe('text-muted')
  })
  it('重大度順（Critical→Info）に並べ替える', () => {
    const input = [{ severity: 'Low' }, { severity: 'Critical' }, { severity: 'Medium' }] as any
    expect(sortFindingsBySeverity(input).map((f) => f.severity)).toEqual(['Critical', 'Medium', 'Low'])
    expect(SEVERITY_ORDER[0]).toBe('Critical')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/utils/severity.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the util**

`web/utils/severity.ts`:
```ts
import type { Finding } from '~/types/goodast'

// 重大度→tokens.css セマンティッククラス（frontend.md「重大度カラー」）。未知は muted。
export function severityTextClass(sev: string): string {
  switch (sev) {
    case 'Critical':
    case 'High':
      return 'text-m-red'
    case 'Medium':
      return 'text-warning'
    default:
      return 'text-muted'
  }
}

export const SEVERITY_ORDER = ['Critical', 'High', 'Medium', 'Low', 'Info']

export function sortFindingsBySeverity(findings: Finding[]): Finding[] {
  const rank = (s: string) => {
    const i = SEVERITY_ORDER.indexOf(s)
    return i === -1 ? SEVERITY_ORDER.length : i
  }
  return [...findings].sort((a, b) => rank(a.severity) - rank(b.severity))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/utils/severity.spec.ts`
Expected: PASS（100%。未知順序の分岐も網羅されるよう、必要なら未知 severity のケースを1件追加）

- [ ] **Step 5: Commit**

```bash
git add web/utils/severity.ts web/tests/utils/severity.spec.ts
git commit -m "feat(web): 重大度→色/順序 util を追加する"
```

---

### Task C2: useScanPolling composable（進捗ポーリング状態機械）

**Files:**
- Create: `web/composables/useScanPolling.ts`
- Test: `web/tests/composables/useScanPolling.spec.ts`

**Interfaces:**
- Produces: `useScanPolling(scanId: string, opts?: { intervalMs?: number })` → `{ state: Ref<ScanState|null>, error: Ref<string|null>, done: Ref<boolean>, start(): void, stop(): void }`
- 挙動: `start` で `GET /scans/:id` を繰り返し、`status` が `done`/`failed` になったら `done=true` にして停止。unmount で停止。

- [ ] **Step 1: Write the failing test**

`web/tests/composables/useScanPolling.spec.ts`（`useApiClient` をモック・`vi.useFakeTimers()`）:
```ts
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const get = vi.fn()
vi.mock('~/composables/useApi', () => ({ useApiClient: () => ({ GET: get }) }))
import { useScanPolling } from '~/composables/useScanPolling'

describe('useScanPolling', () => {
  beforeEach(() => { get.mockReset(); vi.useFakeTimers() })
  afterEach(() => vi.useRealTimers())

  it('running→done で停止し done=true になる', async () => {
    get
      .mockResolvedValueOnce({ data: { id: 'x', status: 'running' }, error: undefined })
      .mockResolvedValueOnce({ data: { id: 'x', status: 'done', summary: { score: 90 } }, error: undefined })
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0)      // 初回即時取得
    expect(p.state.value?.status).toBe('running')
    expect(p.done.value).toBe(false)
    await vi.advanceTimersByTimeAsync(1000)    // 2 回目
    expect(p.state.value?.status).toBe('done')
    expect(p.done.value).toBe(true)
    expect(get.mock.calls.length).toBe(2)      // 停止後は呼ばれない
  })

  it('failed で done=true・状態は failed', async () => {
    get.mockResolvedValueOnce({ data: { id: 'x', status: 'failed' }, error: undefined })
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.done.value).toBe(true)
    expect(p.state.value?.status).toBe('failed')
  })

  it('取得エラーは error にセットし停止する', async () => {
    get.mockResolvedValueOnce({ data: undefined, error: { error: '見つかりません' } })
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.error.value).toContain('見つかりません')
    expect(p.done.value).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/composables/useScanPolling.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the composable**

`web/composables/useScanPolling.ts`:
```ts
import type { ScanState } from '~/types/goodast'

export function useScanPolling(scanId: string, opts: { intervalMs?: number } = {}) {
  const intervalMs = opts.intervalMs ?? 2500
  const client = useApiClient()
  const state = ref<ScanState | null>(null)
  const error = ref<string | null>(null)
  const done = ref(false)
  let timer: ReturnType<typeof setTimeout> | null = null

  function stop() {
    if (timer) { clearTimeout(timer); timer = null }
  }

  async function tick() {
    const { data, error: apiError } = await client.GET('/scans/{id}', { params: { path: { id: scanId } } })
    if (apiError || !data) {
      error.value = toApiErrorMessage(apiError)
      done.value = true
      stop()
      return
    }
    state.value = data
    if (data.status === 'done' || data.status === 'failed') {
      done.value = true
      stop()
      return
    }
    timer = setTimeout(tick, intervalMs)
  }

  function start() {
    done.value = false
    error.value = null
    void tick()
  }

  onUnmounted(stop)
  return { state, error, done, start, stop }
}
```
> `onUnmounted` はコンポーネント外テストでは警告になり得るため、テストは `start`/`stop` を直接検証する（上記テストはそれで通る）。`onUnmounted` が SSR/テスト環境で問題になる場合は `getCurrentInstance()` ガードで囲む。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/composables/useScanPolling.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/composables/useScanPolling.ts web/tests/composables/useScanPolling.spec.ts
git commit -m "feat(web): スキャン進捗ポーリング composable を追加する"
```

---

### Task C3: SeverityBadge + FindingCard + FindingList

**Files:**
- Create: `web/components/scan/SeverityBadge.vue`, `web/components/scan/FindingCard.vue`, `web/components/scan/FindingList.vue`
- Test: `web/tests/components/scan/SeverityBadge.spec.ts`, `FindingCard.spec.ts`, `FindingList.spec.ts`

**Interfaces:**
- Consumes: `severityTextClass`, `sortFindingsBySeverity`（C1）, `Finding`（型）
- Produces: `<SeverityBadge :severity="s" />`, `<FindingCard :finding="f" />`, `<FindingList :findings="f[]" />`（0件は「検出はありませんでした」）

- [ ] **Step 1: Write the failing tests**

`SeverityBadge.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SeverityBadge from '~/components/scan/SeverityBadge.vue'

describe('SeverityBadge', () => {
  it('重大度ラベルを色クラス付きで表示する', () => {
    const w = mount(SeverityBadge, { props: { severity: 'Critical' } })
    expect(w.text()).toContain('Critical')
    expect(w.find('[data-testid="severity-badge"]').classes()).toContain('text-m-red')
  })
})
```
`FindingCard.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FindingCard from '~/components/scan/FindingCard.vue'

const finding = { id: 'f1', template_id: 't1', title: 'XSS', severity: 'High', url: 'http://x/y', cwe: 'CWE-79', remediation: 'エスケープする', status: 'open' }

describe('FindingCard', () => {
  it('タイトル・URL・CWE・修正方法を表示する', () => {
    const w = mount(FindingCard, { props: { finding } })
    expect(w.text()).toContain('XSS')
    expect(w.text()).toContain('http://x/y')
    expect(w.text()).toContain('CWE-79')
    expect(w.text()).toContain('エスケープする')
  })
  it('cwe / remediation が空なら該当行を出さない', () => {
    const w = mount(FindingCard, { props: { finding: { ...finding, cwe: '', remediation: '' } } })
    expect(w.find('[data-testid="finding-cwe"]').exists()).toBe(false)
    expect(w.find('[data-testid="finding-remediation"]').exists()).toBe(false)
  })
})
```
`FindingList.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FindingList from '~/components/scan/FindingList.vue'

const mk = (severity: string, id: string) => ({ id, template_id: id, title: id, severity, url: 'u', cwe: '', remediation: '', status: 'open' })

describe('FindingList', () => {
  it('重大度順に並べて描画する', () => {
    const w = mount(FindingList, { props: { findings: [mk('Low', 'a'), mk('Critical', 'b')] } })
    const cards = w.findAll('[data-testid="finding"]')
    expect(cards).toHaveLength(2)
    expect(cards[0].text()).toContain('b') // Critical 先頭
  })
  it('0 件は「検出はありませんでした」を表示', () => {
    const w = mount(FindingList, { props: { findings: [] } })
    expect(w.text()).toContain('検出はありませんでした')
    expect(w.find('[data-testid="finding"]').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm exec vitest run tests/components/scan/SeverityBadge.spec.ts tests/components/scan/FindingCard.spec.ts tests/components/scan/FindingList.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the components**

`web/components/scan/SeverityBadge.vue`:
```vue
<script setup lang="ts">
const props = defineProps<{ severity: string }>()
const cls = computed(() => severityTextClass(props.severity))
</script>

<template>
  <span data-testid="severity-badge" :class="['text-caption font-bold uppercase tracking-caption', cls]">
    {{ severity }}
  </span>
</template>
```
`web/components/scan/FindingCard.vue`:
```vue
<script setup lang="ts">
import type { Finding } from '~/types/goodast'

defineProps<{ finding: Finding }>()
</script>

<template>
  <article data-testid="finding" class="border border-hairline bg-surface-card p-6">
    <div class="flex items-center justify-between gap-4">
      <p class="font-display text-title-md font-bold text-on-dark">{{ finding.title }}</p>
      <SeverityBadge :severity="finding.severity" />
    </div>
    <p class="mt-2 break-all text-body-sm">{{ finding.url }}</p>
    <p v-if="finding.cwe" data-testid="finding-cwe" class="mt-2 text-caption uppercase tracking-caption text-muted">{{ finding.cwe }}</p>
    <p v-if="finding.remediation" data-testid="finding-remediation" class="mt-3 text-body-sm">
      <span class="text-label font-bold uppercase tracking-label text-muted">修正方法：</span>{{ finding.remediation }}
    </p>
  </article>
</template>
```
`web/components/scan/FindingList.vue`:
```vue
<script setup lang="ts">
import type { Finding } from '~/types/goodast'

const props = defineProps<{ findings: Finding[] }>()
const sorted = computed(() => sortFindingsBySeverity(props.findings))
</script>

<template>
  <div v-if="sorted.length" class="space-y-4">
    <FindingCard v-for="f in sorted" :key="f.id" :finding="f" />
  </div>
  <p v-else class="border border-hairline bg-surface-soft p-6 text-body-sm text-muted">
    検出はありませんでした。
  </p>
</template>
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm exec vitest run tests/components/scan/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/components/scan/SeverityBadge.vue web/components/scan/FindingCard.vue web/components/scan/FindingList.vue web/tests/components/scan/SeverityBadge.spec.ts web/tests/components/scan/FindingCard.spec.ts web/tests/components/scan/FindingList.spec.ts
git commit -m "feat(web): 検出結果（バッジ/カード/リスト）コンポーネントを追加する"
```

---

### Task C4: ScanProgress コンポーネント

**Files:**
- Create: `web/components/scan/ScanProgress.vue`
- Test: `web/tests/components/scan/ScanProgress.spec.ts`

**Interfaces:**
- Produces: `<ScanProgress :status="'queued'|'running'|'failed'" />`（ステップ表示。failed はエラー表現）

- [ ] **Step 1: Write the failing test**

`web/tests/components/scan/ScanProgress.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ScanProgress from '~/components/scan/ScanProgress.vue'

describe('ScanProgress', () => {
  it('running は実行中表示', () => {
    const w = mount(ScanProgress, { props: { status: 'running' } })
    expect(w.text()).toContain('スキャン実行中')
  })
  it('queued は待機中表示', () => {
    const w = mount(ScanProgress, { props: { status: 'queued' } })
    expect(w.text()).toContain('待機中')
  })
  it('failed はエラー表示', () => {
    const w = mount(ScanProgress, { props: { status: 'failed' } })
    expect(w.find('[data-testid="scan-failed"]').exists()).toBe(true)
    expect(w.text()).toContain('失敗')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/components/scan/ScanProgress.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the component**

`web/components/scan/ScanProgress.vue`:
```vue
<script setup lang="ts">
const props = defineProps<{ status: string }>()
const label = computed(() => (props.status === 'queued' ? '待機中…' : 'スキャン実行中…'))
</script>

<template>
  <div v-if="status === 'failed'" data-testid="scan-failed" class="border border-m-red p-6 text-body-sm text-m-red">
    スキャンに失敗しました。時間をおいて再度お試しください。
  </div>
  <div v-else class="border border-hairline bg-surface-soft p-6">
    <p class="animate-pulse font-display text-title-md font-bold text-on-dark">{{ label }}</p>
    <p class="mt-2 text-caption uppercase tracking-caption text-muted">queued → running → done</p>
  </div>
</template>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/components/scan/ScanProgress.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/components/scan/ScanProgress.vue web/tests/components/scan/ScanProgress.spec.ts
git commit -m "feat(web): スキャン進捗表示コンポーネントを追加する"
```

---

### Task C5: /scans/[id] ページ（進捗→結果）

**Files:**
- Create: `web/pages/scans/[id].vue`
- Test: `web/tests/pages/scans-id.spec.ts`

**Interfaces:**
- Consumes: `useScanPolling`（C2）, `ScanProgress`（C4）, `FindingList`（C3）, `ScoreCard`（既存・再利用）, `useApiClient`
- 挙動: マウントで `useScanPolling(id).start()`。`done && status==='done'` で `GET /scans/:id/findings` を取得し結果表示。`failed` は `ScanProgress` の失敗表示。末尾に「専門家への相談」導線。

- [ ] **Step 1: Write the failing test**

`web/tests/pages/scans-id.spec.ts`:
```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import { ref } from 'vue'

const get = vi.fn()
const polling = { state: ref<any>(null), error: ref<string | null>(null), done: ref(false), start: vi.fn(), stop: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApiClient: () => ({ GET: get }) }))
vi.mock('~/composables/useScanPolling', () => ({ useScanPolling: () => polling }))
vi.mock('#app', async (orig) => ({ ...(await orig() as object), useRoute: () => ({ params: { id: 'scan-1' } }) }))

import ScanResult from '~/pages/scans/[id].vue'

describe('pages/scans/[id]', () => {
  beforeEach(() => {
    get.mockReset(); polling.start.mockReset()
    polling.state.value = null; polling.done.value = false; polling.error.value = null
  })

  it('マウントでポーリングを開始し、進捗中は ScanProgress を表示', async () => {
    polling.state.value = { status: 'running' }
    const w = await mountSuspended(ScanResult)
    expect(polling.start).toHaveBeenCalled()
    expect(w.text()).toContain('スキャン実行中')
  })

  it('done で findings を取得して結果を表示する', async () => {
    polling.state.value = { id: 'scan-1', status: 'done', summary: { score: 90, band: 'good', label: '良好', counts: {} } }
    polling.done.value = true
    get.mockResolvedValueOnce({ data: [{ id: 'f1', title: 'XSS', severity: 'High', url: 'u', cwe: '', remediation: '', template_id: 't', status: 'open' }], error: undefined })
    const w = await mountSuspended(ScanResult)
    await new Promise((r) => setTimeout(r))
    expect(get).toHaveBeenCalledWith('/scans/{id}/findings', { params: { path: { id: 'scan-1' } } })
    expect(w.text()).toContain('XSS')
    expect(w.text()).toContain('専門家')
  })

  it('failed は失敗表示', async () => {
    polling.state.value = { status: 'failed' }
    polling.done.value = true
    const w = await mountSuspended(ScanResult)
    expect(w.find('[data-testid="scan-failed"]').exists()).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run tests/pages/scans-id.spec.ts`
Expected: FAIL

- [ ] **Step 3: Write the page**

`web/pages/scans/[id].vue`:
```vue
<script setup lang="ts">
import type { Finding, LatestState } from '~/types/goodast'

const route = useRoute()
const scanId = route.params.id as string
const client = useApiClient()
const { state, done, start } = useScanPolling(scanId)

const findings = ref<Finding[]>([])
const findingsError = ref<string | null>(null)

const isDone = computed(() => done.value && state.value?.status === 'done')
const isFailed = computed(() => state.value?.status === 'failed')

// done になったら明細を取得する
watch(isDone, async (v) => {
  if (!v) return
  const { data, error } = await client.GET('/scans/{id}/findings', { params: { path: { id: scanId } } })
  if (error || !data) { findingsError.value = toApiErrorMessage(error); return }
  findings.value = data
})

// summary → ScoreCard 用 LatestState 形へ寄せる（delta/date は無し）
const latest = computed<LatestState | null>(() => {
  const s = state.value?.summary
  if (!s) return null
  return { scan_id: scanId, score: s.score, band: s.band, label: s.label, counts: s.counts } as LatestState
})

onMounted(start)
</script>

<template>
  <section class="mx-auto max-w-3xl">
    <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">スキャン結果</h1>

    <ScanProgress v-if="!isDone" class="mt-8" :status="state?.status ?? 'queued'" />

    <template v-else>
      <ScoreCard class="mt-8" :latest="latest" />
      <p v-if="findingsError" class="mt-6 border border-m-red p-4 text-body-sm text-m-red">{{ findingsError }}</p>
      <FindingList class="mt-6" :findings="findings" />
      <div class="mt-10 border-t border-hairline pt-6 text-body-sm text-muted">
        より詳しい対策が必要ですか？ <a href="#" data-testid="expert" class="text-on-dark underline">専門家への相談</a> をご検討ください。
      </div>
    </template>
  </section>
</template>
```
> `isFailed` は `ScanProgress`（`status='failed'`）が担うため、`!isDone` 分岐で failed も進捗側に入り失敗表示になる。`isFailed` を明示的に使う必要があれば分岐を足すが、テストは `ScanProgress` の `scan-failed` で通る。未使用変数を残さないよう、`isFailed` を使わない場合は定義を削除する。

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run tests/pages/scans-id.spec.ts`
Expected: PASS

- [ ] **Step 5: Full suite + coverage for PR-C**

Run: `cd web && pnpm test --coverage`
Expected: 全 PASS・Statements/Branches/Functions 100%。未網羅があればテスト追加（特に page の watch 分岐・findings エラー経路）。

- [ ] **Step 6: Commit**

```bash
git add web/pages/scans/\[id\].vue web/tests/pages/scans-id.spec.ts
git commit -m "feat(web): スキャン進捗→結果レポートページを追加する"
```

> **PR-C ここまで**。`make test-web` 緑を確認し PR を作成。3 PR マージで UI 一気通貫が完成。

---

## Self-Review

**Spec coverage:**
- ①サイト登録 + 所有確認ガイド + verify → Task A1/A2/A3 ✅
- `/` の登録導線 → Task A4 ✅
- ②スキャン設定ウィザード（プリセット・危険パス情報表示）→ Task B1/B2/B3 ✅
- `/sites/[id]` の新規スキャン導線・所有未確認バナー → Task B4 ✅
- ③進捗ポーリング → Task C2/C4/C5 ✅
- ④結果レポート（スコア・重大度色・CWE・URL・修正方法・専門家導線・0件表示）→ Task C1/C3/C5 ✅
- 型エイリアス（ScanState/Finding 等）→ Task 0 ✅
- 3 PR 段階化 → PR-A（Task0,A1-A4）/ PR-B（B1-B4）/ PR-C（C1-C5）✅

**Placeholder scan:** 「専門家への相談」は v1 仕様どおり静的リンク（`href="#"`）で明記済み。他にプレースホルダなし。Task A4/B4 の既存 spec ヘルパ名（`mountIndex`/`mountSiteId`）は「既存 spec の手順に合わせる」と明記（実ファイルに合わせて実装）。

**Type consistency:**
- `ScanPresetValue`（`'light'|'standard'|'deep'`）: B1 定義、B2/B3 で一貫使用。`DEFAULT_PRESET='standard'`。
- `severityTextClass`/`sortFindingsBySeverity`/`SEVERITY_ORDER`: C1 定義、C3 で使用。
- `useScanPolling(scanId, {intervalMs})` → `{state,error,done,start,stop}`: C2 定義、C5 で使用。
- 型エイリアス `ScanState`/`Finding`/`ScanSummary`/`SiteResponse`: Task 0 定義、A3/B3/C2/C3/C5 で使用。
- API 呼び出し: `client.POST('/sites',{body})` / `POST('/sites/{id}/verify',{params:{path:{id}}})` / `POST('/scans',{body:{site_id,preset}})` / `GET('/scans/{id}',...)` / `GET('/scans/{id}/findings',...)` — openapi-fetch のパステンプレート表記で一貫。

**実装上の注意（実装者へ）:**
- Task B2 の data-testid セレクタは「共通フックと値別フックの両立」を必ずテストと一致させる（プラン内に明記）。
- Task A4/B4 は既存 spec のマウントヘルパに合わせる（新規ヘルパを作らない）。
- ページの mutation は `useAsyncData` を使わずクライアント直呼び（テストのモックが `POST`/`GET` を直接差し替える前提）。
- `onMounted`/`onUnmounted`/`watch` はコンポーネント文脈でのみ有効。composable の単体テストは `start`/`stop` を直接検証する（プランのテストがその形）。
