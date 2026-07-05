# ダッシュボード（サイト一覧 + スコア + Chart.js）Implementation Plan（PR B）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** サイト一覧（`GET /sites`）→ サイト別ダッシュボード（`GET /sites/:id` + `GET /sites/:id/dashboard`）を実装し、企画書 §6-6 の3層（状態 / 遷移 / 内訳）を Chart.js で描画する。

**Architecture:** 純粋関数（chart config 生成・Band→色マップ・delta 整形）を `utils/` に隔離して unit 100%、canvas 描画は `ChartCanvas.vue` 薄ラッパ（chart.js モックでテスト）、ページは `useAsyncData({ server: false })` + `useApiClient` で取得。色は tokens.css の CSS 変数を実行時に解決して Chart.js へ注入する（canvas は CSS を継承しないため）。

**Tech Stack:** PR A の基盤 + chart.js v4（`pnpm add chart.js`・自作薄ラッパ。vue-chartjs は使わない）

## Global Constraints

- ブランチ: `feat/0020-dashboard`（`chore/0019-web-scaffold` から分岐。PR A マージ後に main へ rebase して PR を出す）
- PR A の Global Constraints をすべて引き継ぐ(生 hex 禁止・`<style>` 禁止・strict・カバレッジ 100%・pnpm・コミットフッター)
- コンポーネントの auto-import 名: `components/dashboard/ScoreCard.vue` → `<DashboardScoreCard>`（pathPrefix 既定）
- `utils/` `composables/` は Nuxt auto-import 対象。テストコード内では明示 import してよい
- スコア色分け（frontend.md）: good→`--color-success` / caution→`--color-warning` / danger・crisis→`--color-m-red`（crisis は opacity 強調）
- 遷移グラフは **history < 2 で「データ不足」空表示**（web/CLAUDE.md）

---

### Task 1: ブランチ作成 + chart.js 追加

**Files:**
- Modify: `web/package.json` / `web/pnpm-lock.yaml`

- [ ] **Step 1: ブランチ作成**

```bash
git checkout chore/0019-web-scaffold && git checkout -b feat/0020-dashboard
```

- [ ] **Step 2: chart.js を追加**

```bash
cd web && pnpm add chart.js
```
Expected: `dependencies` に `chart.js` が追加される。

- [ ] **Step 3: Commit**

```bash
git add web/package.json web/pnpm-lock.yaml
git commit -m "chore(dashboard): chart.js を追加する"
```

---

### Task 2: score-band 純粋ロジック（TDD）

**Files:**
- Create: `web/utils/score-band.ts`
- Test: `web/tests/utils/score-band.spec.ts`

**Interfaces:**
- Produces:
  - `scoreBandStyle(band: string | undefined): ScoreBandStyle`（`{ text: string; emphasis: boolean }`）— Task 5 `ScoreCard.vue` が消費
  - `formatDelta(delta: number | null | undefined): string | null` — 同上

- [ ] **Step 1: 失敗するテストを書く**

`web/tests/utils/score-band.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { formatDelta, scoreBandStyle } from '~/utils/score-band'

describe('scoreBandStyle', () => {
  it.each([
    ['good', 'text-success', false],
    ['caution', 'text-warning', false],
    ['danger', 'text-m-red', false],
    ['crisis', 'text-m-red', true],
    ['unknown-band', 'text-muted', false],
    [undefined, 'text-muted', false],
  ])('band=%s → text=%s emphasis=%s', (band, text, emphasis) => {
    expect(scoreBandStyle(band)).toEqual({ text, emphasis })
  })
})

describe('formatDelta', () => {
  it.each([
    [5, '+5↑'],
    [-12, '-12↓'],
    [0, '±0'],
    [null, null],
    [undefined, null],
  ])('delta=%s → %s', (delta, expected) => {
    expect(formatDelta(delta)).toBe(expected)
  })
})
```

- [ ] **Step 2: 失敗を確認**

```bash
cd web && pnpm test --run tests/utils/score-band.spec.ts
```
Expected: FAIL（モジュール未存在）。

- [ ] **Step 3: 実装**

`web/utils/score-band.ts`:
```ts
export interface ScoreBandStyle {
  /** スコア数字に付けるテキスト色クラス */
  text: string
  /** crisis の opacity 強調（frontend.md「セキュリティスコアの色分け」） */
  emphasis: boolean
}

// backend は Band（セマンティック）を返し、tokens.css の CSS 変数へのマップは
// frontend の責務（PROGRESS.md の責務分離決定）。未知 Band は muted に落とす（前方互換）
export function scoreBandStyle(band: string | undefined): ScoreBandStyle {
  switch (band) {
    case 'good':
      return { text: 'text-success', emphasis: false }
    case 'caution':
      return { text: 'text-warning', emphasis: false }
    case 'danger':
      return { text: 'text-m-red', emphasis: false }
    case 'crisis':
      return { text: 'text-m-red', emphasis: true }
    default:
      return { text: 'text-muted', emphasis: false }
  }
}

// 前回差分の表示形式は企画書 §6-6（例: +5↑ / -12↓）
export function formatDelta(delta: number | null | undefined): string | null {
  if (delta == null) return null
  if (delta > 0) return `+${delta}↑`
  if (delta < 0) return `${delta}↓`
  return '±0'
}
```

- [ ] **Step 4: パスを確認して Commit**

```bash
cd web && pnpm test --run tests/utils/score-band.spec.ts
git add web/utils/score-band.ts web/tests/utils/score-band.spec.ts
git commit -m "feat(dashboard): Band→色クラスと差分表示の純粋ロジックを追加する"
```

---

### Task 3: chart config 生成の純粋関数（TDD）

**Files:**
- Create: `web/utils/chart-config.ts`
- Test: `web/tests/utils/chart-config.spec.ts`

**Interfaces:**
- Consumes: `HistoryEntry`（`~/types/goodast`・PR A Task 5）
- Produces:
  - `MIN_TREND_POINTS = 2` / `hasTrendData(history): boolean`
  - `ChartPalette`（`{ line, grid, text, severity: { critical, high, medium, low, info } }` すべて string）
  - `buildScoreTrendConfig(history, palette): ChartConfiguration<'line'>`
  - `buildSeverityStackConfig(history, palette): ChartConfiguration<'bar'>`

- [ ] **Step 1: 失敗するテストを書く**

`web/tests/utils/chart-config.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import type { HistoryEntry } from '~/types/goodast'
import {
  MIN_TREND_POINTS,
  buildScoreTrendConfig,
  buildSeverityStackConfig,
  hasTrendData,
  type ChartPalette,
} from '~/utils/chart-config'

const palette: ChartPalette = {
  line: 'line-color',
  grid: 'grid-color',
  text: 'text-color',
  severity: {
    critical: 'c-color',
    high: 'h-color',
    medium: 'm-color',
    low: 'l-color',
    info: 'i-color',
  },
}

const fullEntry: HistoryEntry = {
  scan_id: 's-1',
  date: '2026-07-01',
  score: 67,
  band: 'caution',
  counts: { critical: 1, high: 2, medium: 3, low: 4, info: 5, total: 15 },
}

// 生成型は全フィールド optional（swagger 2.0 に required 指定がないため）。
// 欠損時のフォールバック分岐を空オブジェクトで検証する
const emptyEntry: HistoryEntry = {}

describe('hasTrendData', () => {
  it.each([
    [0, false],
    [1, false],
    [2, true],
  ])('history %d 件 → %s', (n, expected) => {
    expect(hasTrendData(Array.from({ length: n }, () => fullEntry))).toBe(expected)
    expect(MIN_TREND_POINTS).toBe(2)
  })
})

describe('buildScoreTrendConfig', () => {
  it('日付ラベルとスコア系列を palette の色で組み立てる', () => {
    const config = buildScoreTrendConfig([fullEntry, { ...fullEntry, date: '2026-07-02', score: 80 }], palette)
    expect(config.type).toBe('line')
    expect(config.data.labels).toEqual(['2026-07-01', '2026-07-02'])
    expect(config.data.datasets[0]!.data).toEqual([67, 80])
    expect(config.data.datasets[0]!.borderColor).toBe('line-color')
    expect(config.options?.scales?.y).toMatchObject({ min: 0, max: 100 })
  })

  it('date/score 欠損は空ラベル・0 にフォールバックする', () => {
    const config = buildScoreTrendConfig([emptyEntry], palette)
    expect(config.data.labels).toEqual([''])
    expect(config.data.datasets[0]!.data).toEqual([0])
  })
})

describe('buildSeverityStackConfig', () => {
  it('重大度 5 系列を stacked bar として palette の色で組み立てる', () => {
    const config = buildSeverityStackConfig([fullEntry], palette)
    expect(config.type).toBe('bar')
    expect(config.data.datasets).toHaveLength(5)
    expect(config.data.datasets.map((d) => d.label)).toEqual(['Critical', 'High', 'Medium', 'Low', 'Info'])
    expect(config.data.datasets.map((d) => d.data[0])).toEqual([1, 2, 3, 4, 5])
    expect(config.data.datasets[0]!.backgroundColor).toBe('c-color')
    expect(config.options?.scales?.x).toMatchObject({ stacked: true })
    expect(config.options?.scales?.y).toMatchObject({ stacked: true })
  })

  it('counts 欠損は全系列 0 にフォールバックする', () => {
    const config = buildSeverityStackConfig([emptyEntry], palette)
    expect(config.data.datasets.map((d) => d.data[0])).toEqual([0, 0, 0, 0, 0])
  })
})
```

- [ ] **Step 2: 失敗を確認**

```bash
cd web && pnpm test --run tests/utils/chart-config.spec.ts
```
Expected: FAIL（モジュール未存在）。

- [ ] **Step 3: 実装**

`web/utils/chart-config.ts`:
```ts
import type { ChartConfiguration } from 'chart.js'
import type { HistoryEntry } from '~/types/goodast'

/** 遷移グラフの描画に必要な最小スキャン数（web/CLAUDE.md: 2 未満は「データ不足」） */
export const MIN_TREND_POINTS = 2

/** Chart.js へ注入する色。canvas は CSS 変数を継承しないため実値を渡す（テストでは任意文字列で注入） */
export interface ChartPalette {
  line: string
  grid: string
  text: string
  severity: {
    critical: string
    high: string
    medium: string
    low: string
    info: string
  }
}

export function hasTrendData(history: readonly HistoryEntry[]): boolean {
  return history.length >= MIN_TREND_POINTS
}

export function buildScoreTrendConfig(
  history: readonly HistoryEntry[],
  palette: ChartPalette,
): ChartConfiguration<'line'> {
  return {
    type: 'line',
    data: {
      labels: history.map((h) => h.date ?? ''),
      datasets: [
        {
          label: 'Goodast Security Score',
          data: history.map((h) => h.score ?? 0),
          borderColor: palette.line,
          backgroundColor: palette.line,
          tension: 0.2,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        y: {
          min: 0,
          max: 100,
          grid: { color: palette.grid },
          ticks: { color: palette.text },
        },
        x: {
          grid: { color: palette.grid },
          ticks: { color: palette.text },
        },
      },
      plugins: { legend: { display: false } },
    },
  }
}

const SEVERITY_KEYS = ['critical', 'high', 'medium', 'low', 'info'] as const

const SEVERITY_LABELS: Record<(typeof SEVERITY_KEYS)[number], string> = {
  critical: 'Critical',
  high: 'High',
  medium: 'Medium',
  low: 'Low',
  info: 'Info',
}

export function buildSeverityStackConfig(
  history: readonly HistoryEntry[],
  palette: ChartPalette,
): ChartConfiguration<'bar'> {
  return {
    type: 'bar',
    data: {
      labels: history.map((h) => h.date ?? ''),
      datasets: SEVERITY_KEYS.map((key) => ({
        label: SEVERITY_LABELS[key],
        data: history.map((h) => h.counts?.[key] ?? 0),
        backgroundColor: palette.severity[key],
        stack: 'findings',
      })),
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { stacked: true, grid: { color: palette.grid }, ticks: { color: palette.text } },
        y: { stacked: true, grid: { color: palette.grid }, ticks: { color: palette.text } },
      },
      plugins: { legend: { labels: { color: palette.text } } },
    },
  }
}
```

- [ ] **Step 4: パスを確認して Commit**

```bash
cd web && pnpm test --run tests/utils/chart-config.spec.ts
git add web/utils/chart-config.ts web/tests/utils/chart-config.spec.ts
git commit -m "feat(dashboard): Chart.js config 生成の純粋関数を追加する"
```

---

### Task 4: パレット解決（CSS 変数 → 実値）+ ChartCanvas 薄ラッパ

**Files:**
- Create: `web/utils/chart-palette.ts`
- Create: `web/components/dashboard/ChartCanvas.vue`
- Test: `web/tests/utils/chart-palette.spec.ts`
- Test: `web/tests/components/dashboard/ChartCanvas.spec.ts`
- Delete: `web/components/dashboard/.gitkeep`

**Interfaces:**
- Consumes: `ChartPalette`（Task 3）
- Produces:
  - `resolveChartPalette(): ChartPalette` — client 専用（`getComputedStyle`）。Task 6 のグラフコンポーネントが消費
  - `<DashboardChartCanvas :config="ChartConfiguration" />` — 同上

> **重大度→色のマッピングはデザイン判断**（frontend.md は findings 用に success/warning/m-red を定めるが、5系列の積み上げ棒の配色は未定義）。既定: critical=`--color-m-red` / high=`--color-warning` / medium=`--color-m-blue-dark` / low=`--color-m-blue-light` / info=`--color-muted`。実装時にユーザーへ確認する（学習モードの貢献ポイント）。

- [ ] **Step 1: 失敗するテストを書く（palette）**

`web/tests/utils/chart-palette.spec.ts`:
```ts
import { afterEach, describe, expect, it } from 'vitest'
import { resolveChartPalette } from '~/utils/chart-palette'

const TOKENS = [
  '--color-m-blue-dark',
  '--color-hairline',
  '--color-body',
  '--color-m-red',
  '--color-warning',
  '--color-m-blue-light',
  '--color-muted',
] as const

describe('resolveChartPalette', () => {
  afterEach(() => {
    for (const t of TOKENS) document.documentElement.style.removeProperty(t)
  })

  it('documentElement の CSS 変数から役割別の色を解決する（trim 込み)', () => {
    const root = document.documentElement
    root.style.setProperty('--color-m-blue-dark', ' rgb(28, 105, 212) ')
    root.style.setProperty('--color-hairline', 'rgb(60, 60, 60)')
    root.style.setProperty('--color-body', 'rgb(187, 187, 187)')
    root.style.setProperty('--color-m-red', 'rgb(226, 39, 24)')
    root.style.setProperty('--color-warning', 'rgb(244, 180, 0)')
    root.style.setProperty('--color-m-blue-light', 'rgb(0, 102, 177)')
    root.style.setProperty('--color-muted', 'rgb(126, 126, 126)')

    expect(resolveChartPalette()).toEqual({
      line: 'rgb(28, 105, 212)',
      grid: 'rgb(60, 60, 60)',
      text: 'rgb(187, 187, 187)',
      severity: {
        critical: 'rgb(226, 39, 24)',
        high: 'rgb(244, 180, 0)',
        medium: 'rgb(28, 105, 212)',
        low: 'rgb(0, 102, 177)',
        info: 'rgb(126, 126, 126)',
      },
    })
  })
})
```

- [ ] **Step 2: 失敗を確認**

```bash
cd web && pnpm test --run tests/utils/chart-palette.spec.ts
```
Expected: FAIL（モジュール未存在）。

- [ ] **Step 3: palette を実装**

`web/utils/chart-palette.ts`:
```ts
import type { ChartPalette } from '~/utils/chart-config'

// canvas 描画は CSS 変数を継承しないため、tokens.css の値を実行時に解決して注入する。
// client 専用（<ClientOnly> 配下でのみ呼ぶこと）
export function resolveChartPalette(): ChartPalette {
  const styles = getComputedStyle(document.documentElement)
  const token = (name: string) => styles.getPropertyValue(name).trim()
  return {
    line: token('--color-m-blue-dark'),
    grid: token('--color-hairline'),
    text: token('--color-body'),
    severity: {
      critical: token('--color-m-red'),
      high: token('--color-warning'),
      medium: token('--color-m-blue-dark'),
      low: token('--color-m-blue-light'),
      info: token('--color-muted'),
    },
  }
}
```

- [ ] **Step 4: palette テストのパスを確認**

```bash
cd web && pnpm test --run tests/utils/chart-palette.spec.ts
```
Expected: PASS。

- [ ] **Step 5: 失敗するテストを書く（ChartCanvas）**

`web/tests/components/dashboard/ChartCanvas.spec.ts`:
```ts
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { ChartConfiguration } from 'chart.js'
import ChartCanvas from '~/components/dashboard/ChartCanvas.vue'

const { MockChart, instances } = vi.hoisted(() => {
  const created: Array<{
    canvas: unknown
    config: { data: unknown }
    data: unknown
    update: ReturnType<typeof vi.fn>
    destroy: ReturnType<typeof vi.fn>
  }> = []
  class MockChartImpl {
    data: unknown
    update = vi.fn()
    destroy = vi.fn()
    constructor(
      public canvas: unknown,
      public config: { data: unknown },
    ) {
      this.data = config.data
      created.push(this)
    }
  }
  return { MockChart: MockChartImpl, instances: created }
})

vi.mock('chart.js/auto', () => ({ default: MockChart }))

function lineConfig(labels: string[]): ChartConfiguration {
  return { type: 'line', data: { labels, datasets: [] } }
}

describe('DashboardChartCanvas', () => {
  it('mount で canvas に Chart を生成し、config 差し替えで update、unmount で destroy する', async () => {
    const wrapper = mount(ChartCanvas, { props: { config: lineConfig(['a']) } })
    expect(instances).toHaveLength(1)
    const chart = instances[0]!
    expect(chart.canvas).toBe(wrapper.find('canvas').element)

    const next = lineConfig(['a', 'b'])
    await wrapper.setProps({ config: next })
    expect(chart.data).toBe(next.data)
    expect(chart.update).toHaveBeenCalledTimes(1)

    wrapper.unmount()
    expect(chart.destroy).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 6: 失敗を確認**

```bash
cd web && pnpm test --run tests/components/dashboard/ChartCanvas.spec.ts
```
Expected: FAIL（コンポーネント未存在）。

- [ ] **Step 7: ChartCanvas.vue を実装、.gitkeep を削除**

`web/components/dashboard/ChartCanvas.vue`:
```vue
<script setup lang="ts">
import Chart from 'chart.js/auto'
import type { ChartConfiguration } from 'chart.js'

const props = defineProps<{ config: ChartConfiguration }>()

const canvasRef = ref<HTMLCanvasElement>()

// Chart インスタンスはリアクティブにしない（Chart.js 内部状態を Vue が proxy 化すると壊れる）
let chart: Chart

onMounted(() => {
  chart = new Chart(canvasRef.value!, props.config)
})

watch(
  () => props.config,
  (config) => {
    chart.data = config.data
    chart.update()
  },
)

onUnmounted(() => {
  chart.destroy()
})
</script>

<template>
  <canvas ref="canvasRef" />
</template>
```

```bash
rm web/components/dashboard/.gitkeep
```

- [ ] **Step 8: 全テストのパスを確認して Commit**

```bash
cd web && pnpm test --run --coverage
git add web/utils/chart-palette.ts web/components/dashboard/ChartCanvas.vue web/tests/utils/chart-palette.spec.ts web/tests/components/dashboard/ChartCanvas.spec.ts
git rm web/components/dashboard/.gitkeep
git commit -m "feat(dashboard): CSS 変数パレット解決と Chart.js 薄ラッパを追加する"
```

---

### Task 5: ScoreCard + SeverityCountCards（上段・状態）

**Files:**
- Create: `web/components/dashboard/ScoreCard.vue`
- Create: `web/components/dashboard/SeverityCountCards.vue`
- Test: `web/tests/components/dashboard/ScoreCard.spec.ts`
- Test: `web/tests/components/dashboard/SeverityCountCards.spec.ts`

**Interfaces:**
- Consumes: `LatestState` / `SeverityCounts`（`~/types/goodast`）、`scoreBandStyle` / `formatDelta`（Task 2）
- Produces: `<DashboardScoreCard :latest="LatestState | null" />` / `<DashboardSeverityCountCards :counts="SeverityCounts" />` — Task 7 のページが消費

- [ ] **Step 1: 失敗するテストを書く（ScoreCard）**

`web/tests/components/dashboard/ScoreCard.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { LatestState } from '~/types/goodast'
import ScoreCard from '~/components/dashboard/ScoreCard.vue'

const latest: LatestState = {
  scan_id: 's-1',
  date: '2026-07-01',
  score: 67,
  band: 'caution',
  label: '要注意',
  delta: 5,
  counts: { total: 6 },
}

describe('DashboardScoreCard', () => {
  it('スコア・差分・ラベル・日付を Band 色で描画する', () => {
    const wrapper = mount(ScoreCard, { props: { latest } })
    expect(wrapper.text()).toContain('67')
    expect(wrapper.text()).toContain('+5↑')
    expect(wrapper.text()).toContain('要注意')
    expect(wrapper.text()).toContain('2026-07-01')
    expect(wrapper.find('[data-testid="score-value"]').classes()).toContain('text-warning')
  })

  it('delta なし・date なしでは差分・日付を表示しない', () => {
    const wrapper = mount(ScoreCard, {
      props: { latest: { ...latest, delta: undefined, date: undefined } },
    })
    expect(wrapper.text()).not.toContain('↑')
    expect(wrapper.find('[data-testid="score-date"]').exists()).toBe(false)
  })

  it('crisis は opacity 強調（animate-pulse）を付ける', () => {
    const wrapper = mount(ScoreCard, { props: { latest: { ...latest, band: 'crisis' } } })
    const score = wrapper.find('[data-testid="score-value"]')
    expect(score.classes()).toContain('text-m-red')
    expect(score.classes()).toContain('animate-pulse')
  })

  it('latest が null なら「スキャン未実行」を表示する', () => {
    const wrapper = mount(ScoreCard, { props: { latest: null } })
    expect(wrapper.text()).toContain('スキャン未実行')
    expect(wrapper.find('[data-testid="score-value"]').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: 失敗を確認**

```bash
cd web && pnpm test --run tests/components/dashboard/ScoreCard.spec.ts
```
Expected: FAIL（コンポーネント未存在）。

- [ ] **Step 3: ScoreCard.vue を実装**

```vue
<script setup lang="ts">
import type { LatestState } from '~/types/goodast'

const props = defineProps<{ latest: LatestState | null }>()

const band = computed(() => scoreBandStyle(props.latest?.band))
const delta = computed(() => formatDelta(props.latest?.delta))
</script>

<template>
  <section class="bg-surface-card p-6">
    <h2 class="font-display text-label font-bold uppercase tracking-label text-muted">
      Goodast Security Score
    </h2>
    <template v-if="latest">
      <p class="mt-2 flex items-baseline gap-3">
        <span
          data-testid="score-value"
          class="font-display text-display-lg font-bold"
          :class="[band.text, { 'animate-pulse': band.emphasis }]"
        >
          {{ latest.score }}
        </span>
        <span v-if="delta" class="text-title-md text-body-strong">{{ delta }}</span>
      </p>
      <p class="mt-1 text-body-sm">
        {{ latest.label }}
        <span v-if="latest.date" data-testid="score-date" class="ml-2 text-caption text-muted">
          {{ latest.date }}
        </span>
      </p>
    </template>
    <p v-else class="mt-2 text-body-sm text-muted">スキャン未実行</p>
  </section>
</template>
```

- [ ] **Step 4: ScoreCard テストのパスを確認**

```bash
cd web && pnpm test --run tests/components/dashboard/ScoreCard.spec.ts
```
Expected: PASS。

- [ ] **Step 5: 失敗するテストを書く（SeverityCountCards）**

`web/tests/components/dashboard/SeverityCountCards.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SeverityCountCards from '~/components/dashboard/SeverityCountCards.vue'

describe('DashboardSeverityCountCards', () => {
  it('重大度 5 区分のカウントを順に描画する', () => {
    const wrapper = mount(SeverityCountCards, {
      props: { counts: { critical: 1, high: 2, medium: 3, low: 4, info: 5, total: 15 } },
    })
    const cells = wrapper.findAll('[data-testid="severity-cell"]')
    expect(cells).toHaveLength(5)
    expect(cells.map((c) => c.text())).toEqual([
      '1Critical',
      '2High',
      '3Medium',
      '4Low',
      '5Info',
    ])
  })

  it('欠損フィールドは 0 として描画する', () => {
    const wrapper = mount(SeverityCountCards, { props: { counts: {} } })
    const cells = wrapper.findAll('[data-testid="severity-cell"]')
    expect(cells.map((c) => c.text())).toEqual(['0Critical', '0High', '0Medium', '0Low', '0Info'])
  })
})
```

- [ ] **Step 6: 失敗を確認**

```bash
cd web && pnpm test --run tests/components/dashboard/SeverityCountCards.spec.ts
```
Expected: FAIL（コンポーネント未存在）。

- [ ] **Step 7: SeverityCountCards.vue を実装**

```vue
<script setup lang="ts">
import type { SeverityCounts } from '~/types/goodast'

const props = defineProps<{ counts: SeverityCounts }>()

const items = computed(() => [
  { label: 'Critical', value: props.counts.critical ?? 0 },
  { label: 'High', value: props.counts.high ?? 0 },
  { label: 'Medium', value: props.counts.medium ?? 0 },
  { label: 'Low', value: props.counts.low ?? 0 },
  { label: 'Info', value: props.counts.info ?? 0 },
])
</script>

<template>
  <div class="grid grid-cols-2 gap-4 md:grid-cols-5">
    <div
      v-for="item in items"
      :key="item.label"
      data-testid="severity-cell"
      class="bg-surface-soft p-6"
    >
      <p class="font-display text-display-sm font-bold text-on-dark">{{ item.value }}</p>
      <p class="mt-1 text-label font-bold uppercase tracking-label text-muted">{{ item.label }}</p>
    </div>
  </div>
</template>
```

- [ ] **Step 8: 全テストのパスを確認して Commit**

```bash
cd web && pnpm test --run --coverage
git add web/components/dashboard/ScoreCard.vue web/components/dashboard/SeverityCountCards.vue web/tests/components/dashboard/
git commit -m "feat(dashboard): スコアカードと重大度サマリカードを追加する（§6-6 上段）"
```

---

### Task 6: ScoreTrendChart + SeverityStackChart（中段・下段）

**Files:**
- Create: `web/components/dashboard/ScoreTrendChart.vue`
- Create: `web/components/dashboard/SeverityStackChart.vue`
- Test: `web/tests/components/dashboard/ScoreTrendChart.spec.ts`
- Test: `web/tests/components/dashboard/SeverityStackChart.spec.ts`

**Interfaces:**
- Consumes: `hasTrendData` / `buildScoreTrendConfig` / `buildSeverityStackConfig`（Task 3）、`resolveChartPalette`（Task 4）、`<DashboardChartCanvas>`（Task 4）
- Produces: `<DashboardScoreTrendChart :history="HistoryEntry[]" />` / `<DashboardSeverityStackChart :history="HistoryEntry[]" />` — Task 7 のページが消費

- [ ] **Step 1: 失敗するテストを書く**

`web/tests/components/dashboard/ScoreTrendChart.spec.ts`:
```ts
import { describe, expect, it, vi } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import type { HistoryEntry } from '~/types/goodast'
import ScoreTrendChart from '~/components/dashboard/ScoreTrendChart.vue'
import ChartCanvas from '~/components/dashboard/ChartCanvas.vue'

vi.mock('chart.js/auto', () => ({
  default: class {
    data: unknown
    update = vi.fn()
    destroy = vi.fn()
    constructor(_canvas: unknown, config: { data: unknown }) {
      this.data = config.data
    }
  },
}))

const entry = (date: string, score: number): HistoryEntry => ({ date, score, counts: {} })

describe('DashboardScoreTrendChart', () => {
  it('history 2 件以上で折れ線チャートを描画する', async () => {
    const wrapper = await mountSuspended(ScoreTrendChart, {
      props: { history: [entry('2026-07-01', 60), entry('2026-07-02', 70)] },
    })
    const canvas = wrapper.findComponent(ChartCanvas)
    expect(canvas.exists()).toBe(true)
    expect(canvas.props('config').type).toBe('line')
    expect(wrapper.text()).not.toContain('データ不足')
  })

  it('history 2 件未満は「データ不足」を表示しチャートを出さない', async () => {
    const wrapper = await mountSuspended(ScoreTrendChart, {
      props: { history: [entry('2026-07-01', 60)] },
    })
    expect(wrapper.findComponent(ChartCanvas).exists()).toBe(false)
    expect(wrapper.text()).toContain('データ不足')
  })
})
```

`web/tests/components/dashboard/SeverityStackChart.spec.ts`:
```ts
import { describe, expect, it, vi } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import type { HistoryEntry } from '~/types/goodast'
import SeverityStackChart from '~/components/dashboard/SeverityStackChart.vue'
import ChartCanvas from '~/components/dashboard/ChartCanvas.vue'

vi.mock('chart.js/auto', () => ({
  default: class {
    data: unknown
    update = vi.fn()
    destroy = vi.fn()
    constructor(_canvas: unknown, config: { data: unknown }) {
      this.data = config.data
    }
  },
}))

const entry = (date: string): HistoryEntry => ({ date, score: 50, counts: { high: 1 } })

describe('DashboardSeverityStackChart', () => {
  it('history 2 件以上で積み上げ棒チャートを描画する', async () => {
    const wrapper = await mountSuspended(SeverityStackChart, {
      props: { history: [entry('2026-07-01'), entry('2026-07-02')] },
    })
    const canvas = wrapper.findComponent(ChartCanvas)
    expect(canvas.exists()).toBe(true)
    expect(canvas.props('config').type).toBe('bar')
  })

  it('history 2 件未満は「データ不足」を表示する', async () => {
    const wrapper = await mountSuspended(SeverityStackChart, { props: { history: [] } })
    expect(wrapper.findComponent(ChartCanvas).exists()).toBe(false)
    expect(wrapper.text()).toContain('データ不足')
  })
})
```

- [ ] **Step 2: 失敗を確認**

```bash
cd web && pnpm test --run tests/components/dashboard/ScoreTrendChart.spec.ts tests/components/dashboard/SeverityStackChart.spec.ts
```
Expected: FAIL（コンポーネント未存在）。

- [ ] **Step 3: 実装**

`web/components/dashboard/ScoreTrendChart.vue`:
```vue
<script setup lang="ts">
import type { HistoryEntry } from '~/types/goodast'

const props = defineProps<{ history: HistoryEntry[] }>()

const enough = computed(() => hasTrendData(props.history))
// config は <ClientOnly> 配下でのみ評価される（resolveChartPalette は client 専用）
const config = computed(() => buildScoreTrendConfig(props.history, resolveChartPalette()))
</script>

<template>
  <section class="bg-surface-card p-6">
    <h2 class="font-display text-label font-bold uppercase tracking-label text-muted">
      スコア推移
    </h2>
    <div v-if="enough" class="mt-4 h-64">
      <ClientOnly>
        <DashboardChartCanvas :config="config" />
      </ClientOnly>
    </div>
    <p v-else class="mt-4 text-body-sm text-muted">
      データ不足 — 2回以上のスキャンで推移が表示されます
    </p>
  </section>
</template>
```

`web/components/dashboard/SeverityStackChart.vue`:
```vue
<script setup lang="ts">
import type { HistoryEntry } from '~/types/goodast'

const props = defineProps<{ history: HistoryEntry[] }>()

const enough = computed(() => hasTrendData(props.history))
// config は <ClientOnly> 配下でのみ評価される（resolveChartPalette は client 専用）
const config = computed(() => buildSeverityStackConfig(props.history, resolveChartPalette()))
</script>

<template>
  <section class="bg-surface-card p-6">
    <h2 class="font-display text-label font-bold uppercase tracking-label text-muted">
      重大度別内訳
    </h2>
    <div v-if="enough" class="mt-4 h-64">
      <ClientOnly>
        <DashboardChartCanvas :config="config" />
      </ClientOnly>
    </div>
    <p v-else class="mt-4 text-body-sm text-muted">
      データ不足 — 2回以上のスキャンで内訳の推移が表示されます
    </p>
  </section>
</template>
```

- [ ] **Step 4: パスを確認して Commit**

```bash
cd web && pnpm test --run --coverage
git add web/components/dashboard/ScoreTrendChart.vue web/components/dashboard/SeverityStackChart.vue web/tests/components/dashboard/
git commit -m "feat(dashboard): スコア推移と重大度内訳のチャートを追加する（§6-6 中段・下段）"
```

---

### Task 7: エラーメッセージ集約 + サイト一覧ページ

**Files:**
- Create: `web/composables/useApiError.ts`
- Modify: `web/pages/index.vue`（プレースホルダを実装に置換）
- Test: `web/tests/composables/useApiError.spec.ts`
- Modify: `web/tests/pages/index.spec.ts`（全面書き換え）

**Interfaces:**
- Consumes: `useApiClient`（PR A Task 5）、`Site` / `ApiErrorResponse`（`~/types/goodast`）
- Produces: `toApiErrorMessage(error: unknown): string` — Task 8 のダッシュボードページも消費

- [ ] **Step 1: 失敗するテストを書く（useApiError）**

`web/tests/composables/useApiError.spec.ts`:
```ts
import { describe, expect, it } from 'vitest'
import { toApiErrorMessage } from '~/composables/useApiError'

describe('toApiErrorMessage', () => {
  it.each([
    [{ error: 'site not found' }, 'site not found'],
    [{ error: '' }, 'APIとの通信に失敗しました。時間をおいて再度お試しください。'],
    [{ message: 'not-api-shape' }, 'APIとの通信に失敗しました。時間をおいて再度お試しください。'],
    [null, 'APIとの通信に失敗しました。時間をおいて再度お試しください。'],
    ['plain string', 'APIとの通信に失敗しました。時間をおいて再度お試しください。'],
  ])('%j → %s', (input, expected) => {
    expect(toApiErrorMessage(input)).toBe(expected)
  })
})
```

- [ ] **Step 2: 失敗を確認**

```bash
cd web && pnpm test --run tests/composables/useApiError.spec.ts
```
Expected: FAIL（モジュール未存在）。

- [ ] **Step 3: useApiError を実装**

`web/composables/useApiError.ts`:
```ts
const FALLBACK_MESSAGE = 'APIとの通信に失敗しました。時間をおいて再度お試しください。'

// API エラー（openapi-fetch の error = ErrorResponse）を表示用メッセージへ変換する。
// エラーハンドリングの集約点（.claude/rules/frontend.md「API 連携」）
export function toApiErrorMessage(error: unknown): string {
  if (typeof error === 'object' && error !== null && 'error' in error) {
    const message = (error as { error: unknown }).error
    if (typeof message === 'string' && message !== '') return message
  }
  return FALLBACK_MESSAGE
}
```

- [ ] **Step 4: 失敗するテストを書く（サイト一覧ページ）**

`web/tests/pages/index.spec.ts` を全面書き換え:
```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import type { Site } from '~/types/goodast'
import IndexPage from '~/pages/index.vue'

const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }))

mockNuxtImport('useApiClient', () => () => ({ GET: getMock }))

const sites: Site[] = [
  {
    id: '11111111-1111-1111-1111-111111111111',
    name: 'コーポレートサイト',
    base_url: 'https://example.com',
    ownership_verified: true,
  },
  {
    id: '22222222-2222-2222-2222-222222222222',
    name: 'ローカル検証',
    base_url: 'http://localhost:3001',
    ownership_verified: false,
  },
]

describe('pages/index', () => {
  beforeEach(() => {
    getMock.mockReset()
  })

  it('サイトカードを一覧描画しダッシュボードへリンクする', async () => {
    getMock.mockResolvedValue({ data: sites, error: undefined })
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain('コーポレートサイト')
    expect(wrapper.text()).toContain('https://example.com')
    expect(wrapper.text()).toContain('所有確認済み')
    expect(wrapper.text()).toContain('所有未確認')
    expect(
      wrapper.find('a[href="/sites/11111111-1111-1111-1111-111111111111"]').exists(),
    ).toBe(true)
  })

  it('0 件なら未登録の空状態を表示する', async () => {
    getMock.mockResolvedValue({ data: [], error: undefined })
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain('サイトが未登録です')
  })

  it('data が undefined でも空状態にフォールバックする', async () => {
    getMock.mockResolvedValue({ data: undefined, error: undefined })
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain('サイトが未登録です')
  })

  it('API エラーはエラーバンドを表示する', async () => {
    getMock.mockResolvedValue({ data: undefined, error: { error: 'boom' } })
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain('boom')
  })
})
```

- [ ] **Step 5: 失敗を確認**

```bash
cd web && pnpm test --run tests/pages/index.spec.ts
```
Expected: FAIL(プレースホルダのままなので assertion 失敗)。

- [ ] **Step 6: pages/index.vue を実装**

```vue
<script setup lang="ts">
const client = useApiClient()

// SSR では相対 apiBase を解決できないため client 側でのみ取得（composables/useApi.ts 参照）
const { data: sites, error } = await useAsyncData(
  'sites',
  async () => {
    const { data, error: apiError } = await client.GET('/sites')
    if (apiError) throw new Error(toApiErrorMessage(apiError))
    return data ?? []
  },
  { server: false, default: () => [] },
)
</script>

<template>
  <section>
    <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">Sites</h1>
    <p v-if="error" class="mt-6 border border-m-red p-4 text-body-sm text-m-red">
      {{ error.message }}
    </p>
    <p v-else-if="sites.length === 0" class="mt-6 text-body-sm text-muted">
      サイトが未登録です。登録画面は次のリリースで追加予定です（現在は API 経由で登録できます）。
    </p>
    <ul v-else class="mt-6 grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <li v-for="site in sites" :key="site.id">
        <NuxtLink
          :to="`/sites/${site.id}`"
          class="block border border-hairline bg-surface-card p-6 transition-colors hover:border-on-dark"
        >
          <p class="font-display text-title-lg font-bold text-on-dark">{{ site.name }}</p>
          <p class="mt-1 text-body-sm">{{ site.base_url }}</p>
          <p
            class="mt-3 text-caption uppercase tracking-caption"
            :class="site.ownership_verified ? 'text-success' : 'text-warning'"
          >
            {{ site.ownership_verified ? '所有確認済み' : '所有未確認' }}
          </p>
        </NuxtLink>
      </li>
    </ul>
  </section>
</template>
```

- [ ] **Step 7: パスを確認して Commit**

```bash
cd web && pnpm test --run --coverage
git add web/composables/useApiError.ts web/pages/index.vue web/tests/composables/useApiError.spec.ts web/tests/pages/index.spec.ts
git commit -m "feat(dashboard): サイト一覧ページと API エラーメッセージ集約を実装する"
```

---

### Task 8: サイト別ダッシュボードページ（§6-6 組み立て）

**Files:**
- Create: `web/pages/sites/[id].vue`
- Test: `web/tests/pages/sites-id.spec.ts`

**Interfaces:**
- Consumes: `useApiClient` / `toApiErrorMessage` / 全 dashboard コンポーネント（Task 4〜7）

- [ ] **Step 1: 失敗するテストを書く**

`web/tests/pages/sites-id.spec.ts`:
```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import type { DashboardData, Site } from '~/types/goodast'
import SiteDashboardPage from '~/pages/sites/[id].vue'
import ScoreCard from '~/components/dashboard/ScoreCard.vue'
import SeverityCountCards from '~/components/dashboard/SeverityCountCards.vue'
import ScoreTrendChart from '~/components/dashboard/ScoreTrendChart.vue'
import SeverityStackChart from '~/components/dashboard/SeverityStackChart.vue'

vi.mock('chart.js/auto', () => ({
  default: class {
    data: unknown
    update = vi.fn()
    destroy = vi.fn()
    constructor(_canvas: unknown, config: { data: unknown }) {
      this.data = config.data
    }
  },
}))

const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }))

mockNuxtImport('useApiClient', () => () => ({ GET: getMock }))

const SITE_ID = '11111111-1111-1111-1111-111111111111'
const ROUTE = `/sites/${SITE_ID}`

const site: Site = { id: SITE_ID, name: 'コーポレートサイト', base_url: 'https://example.com' }

const dashboard: DashboardData = {
  latest: {
    scan_id: 's-2',
    date: '2026-07-02',
    score: 72,
    band: 'caution',
    label: '要注意',
    delta: 5,
    counts: { critical: 0, high: 1, medium: 3, low: 2, info: 0, total: 6 },
  },
  history: [
    { scan_id: 's-1', date: '2026-07-01', score: 67, band: 'caution', counts: { high: 2, total: 2 } },
    { scan_id: 's-2', date: '2026-07-02', score: 72, band: 'caution', counts: { high: 1, total: 1 } },
  ],
}

function mockApi(overrides?: { site?: unknown; dashboard?: unknown; siteError?: unknown }) {
  getMock.mockImplementation(async (path: string) => {
    if (path === '/sites/{id}') {
      return { data: overrides?.siteError ? undefined : (overrides?.site ?? site), error: overrides?.siteError }
    }
    return { data: overrides?.dashboard ?? dashboard, error: undefined }
  })
}

describe('pages/sites/[id]', () => {
  beforeEach(() => {
    getMock.mockReset()
  })

  it('サイト名と 3 層（スコア/サマリ/チャート×2）を描画する', async () => {
    mockApi()
    const wrapper = await mountSuspended(SiteDashboardPage, { route: ROUTE })
    expect(wrapper.text()).toContain('コーポレートサイト')
    expect(wrapper.findComponent(ScoreCard).exists()).toBe(true)
    expect(wrapper.findComponent(SeverityCountCards).exists()).toBe(true)
    expect(wrapper.findComponent(ScoreTrendChart).exists()).toBe(true)
    expect(wrapper.findComponent(SeverityStackChart).exists()).toBe(true)
    expect(wrapper.text()).toContain('72')
  })

  it('スキャン未実行（latest=null・history=[]）はサマリカードを出さず空状態で描画する', async () => {
    mockApi({ dashboard: { latest: null, history: [] } })
    const wrapper = await mountSuspended(SiteDashboardPage, { route: ROUTE })
    expect(wrapper.text()).toContain('スキャン未実行')
    expect(wrapper.findComponent(SeverityCountCards).exists()).toBe(false)
    expect(wrapper.text()).toContain('データ不足')
  })

  it('dashboard フィールド欠損（undefined）でも空配列へフォールバックする', async () => {
    mockApi({ dashboard: {} })
    const wrapper = await mountSuspended(SiteDashboardPage, { route: ROUTE })
    expect(wrapper.findComponent(ScoreCard).exists()).toBe(true)
    expect(wrapper.text()).toContain('データ不足')
  })

  it('サイト取得エラーはエラーバンドを表示する', async () => {
    mockApi({ siteError: { error: 'site not found' } })
    const wrapper = await mountSuspended(SiteDashboardPage, { route: ROUTE })
    expect(wrapper.text()).toContain('site not found')
    expect(wrapper.findComponent(ScoreCard).exists()).toBe(false)
  })
})
```

- [ ] **Step 2: 失敗を確認**

```bash
cd web && pnpm test --run tests/pages/sites-id.spec.ts
```
Expected: FAIL（ページ未存在）。

- [ ] **Step 3: ページを実装**

`web/pages/sites/[id].vue`:
```vue
<script setup lang="ts">
const route = useRoute()
const siteId = String(route.params.id)
const client = useApiClient()

// SSR では相対 apiBase を解決できないため client 側でのみ取得（composables/useApi.ts 参照）
const { data, error } = await useAsyncData(
  `site-dashboard-${siteId}`,
  async () => {
    const [siteRes, dashboardRes] = await Promise.all([
      client.GET('/sites/{id}', { params: { path: { id: siteId } } }),
      client.GET('/sites/{id}/dashboard', { params: { path: { id: siteId } } }),
    ])
    const apiError = siteRes.error ?? dashboardRes.error
    if (apiError) throw new Error(toApiErrorMessage(apiError))
    return { site: siteRes.data, dashboard: dashboardRes.data }
  },
  { server: false },
)

const site = computed(() => data.value?.site)
const latest = computed(() => data.value?.dashboard?.latest ?? null)
const history = computed(() => data.value?.dashboard?.history ?? [])
</script>

<template>
  <p v-if="error" class="border border-m-red p-4 text-body-sm text-m-red">
    {{ error.message }}
  </p>
  <section v-else-if="site">
    <NuxtLink to="/" class="text-caption uppercase tracking-caption text-muted hover:text-on-dark">
      ← Sites
    </NuxtLink>
    <h1 class="mt-2 font-display text-display-sm font-bold uppercase text-on-dark">
      {{ site.name }}
    </h1>
    <p class="mt-1 text-body-sm text-muted">{{ site.base_url }}</p>
    <div class="m-stripe mt-4" />

    <!-- 上段: 状態（今どうか） -->
    <div class="mt-8 grid gap-4 lg:grid-cols-[minmax(0,20rem)_1fr]">
      <DashboardScoreCard :latest="latest" />
      <DashboardSeverityCountCards v-if="latest?.counts" :counts="latest.counts" />
    </div>

    <!-- 中段: 遷移（どう変わったか）/ 下段: 内訳（なぜそのスコアか） -->
    <div class="mt-8 flex flex-col gap-8">
      <DashboardScoreTrendChart :history="history" />
      <DashboardSeverityStackChart :history="history" />
    </div>
  </section>
  <p v-else class="text-body-sm text-muted">読み込み中…</p>
</template>
```

> **スペックからの意図的な変更**: 404 を `createError` の専用ページにせず、他のエラーと同じエラーバンドで backend のメッセージ（`site not found`）を表示する。`fatal: true` の client エラーはテスト困難で、PoC ではバンド表示で十分なため。

- [ ] **Step 4: パスを確認して Commit**

```bash
cd web && pnpm test --run --coverage
git add web/pages/sites/ web/tests/pages/sites-id.spec.ts
git commit -m "feat(dashboard): サイト別ダッシュボードページを実装する（§6-6）"
```

---

### Task 9: 検証 + PROGRESS 更新 + PR B 作成

- [ ] **Step 1: 全ゲートを実行**

```bash
make test-web
```
Expected: lint / type-check / vitest（カバレッジ Statements・Branches・Functions 100%）すべてパス。

- [ ] **Step 2: 手動スモーク（任意・DB/API 起動環境がある場合）**

```bash
make db-up && make migrate && make dev-api   # 別ターミナル
make dev-web                                  # 別ターミナル
# サイト登録（localhost は即 verified）
curl -sX POST localhost:8080/sites -H 'Content-Type: application/json' \
  -d '{"name":"ローカル検証","base_url":"http://localhost:3001"}'
```
`http://localhost:3000` でサイトカード → クリックでダッシュボード（スキャン未実行状態）を確認。

- [ ] **Step 3: PROGRESS.md を更新**

- 現在地スナップショットに追記: 「**ダッシュボード frontend: 完了**（PR B）— サイト一覧（`GET /sites`）→ サイト別ダッシュボード（`/sites/[id]`・§6-6 の3層: ScoreCard＋SeverityCountCards / ScoreTrendChart / SeverityStackChart）。chart config は純粋関数（unit 100%）・canvas は薄ラッパ＋モック・色は tokens.css の CSS 変数を実行時解決で注入。history<2 は「データ不足」。**残: サイト登録・スキャン実行・結果レポート・履歴の各画面**」
- ロードマップ `- [~] ダッシュボード（スコア + 時系列・Chart.js）` を `[x]` に変更し、「残（frontend・別セッション）」の行を削除
- 最終更新日を更新

- [ ] **Step 4: セルフレビュー + push + PR 作成**

```bash
git diff chore/0019-web-scaffold...HEAD
git add PROGRESS.md && git commit -m "docs: PROGRESS をダッシュボード frontend 完了に更新する"
git push -u origin feat/0020-dashboard
```

PR A マージ後に main へ rebase してから PR を出す:
```bash
git fetch origin && git rebase origin/main
git push --force-with-lease
gh pr create --title "feat(dashboard): サイト一覧とダッシュボード（スコア + Chart.js）を実装する" --body "<下記本文>"
```

PR 本文:
```markdown
## 概要
PoC の中心要件 §6-6 を実装する。サイト一覧からサイト別ダッシュボードへ遷移し、Goodast Security Score（Band 色 + 前回差分）・重大度サマリ・スコア推移折れ線・重大度別積み上げ棒を表示する。

## 変更内容
- `pages/index.vue`: サイト一覧（`GET /sites`・所有確認バッジ・空状態）
- `pages/sites/[id].vue`: ダッシュボード 3 層組み立て（`GET /sites/:id` + `/dashboard`）
- `utils/`: chart config 生成・Band→色マップ・delta 整形（純粋関数・unit 100%）
- `components/dashboard/`: ChartCanvas 薄ラッパ（chart.js モックでテスト）+ 4 コンポーネント
- 色は tokens.css の CSS 変数を実行時解決して Chart.js へ注入（ダークテーマ・生 hex なし）
- history < 2 は「データ不足」空表示（web/CLAUDE.md 準拠）

## 動作確認
- `make test-web`（lint / type-check / vitest カバレッジ 100%）パス
- 手動: api + web 起動 → サイト登録 → 一覧 → ダッシュボード表示（スキャン未実行の空状態）

## 関連 Issue
（設計スペック: docs/superpowers/specs/2026-07-04-web-scaffold-dashboard-design.md）
```
