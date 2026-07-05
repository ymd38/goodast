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

  // 追加: エラーも投げず site データも取得できないケース（例: 204 相当の空応答）。
  // istanbul の branch カバレッジ（v-else の「読み込み中…」フォールバック）を満たすため追加。
  it('エラーなし・site データなしの場合は読み込み中を表示する', async () => {
    getMock.mockImplementation(async () => ({ data: undefined, error: undefined }))
    const wrapper = await mountSuspended(SiteDashboardPage, { route: ROUTE })
    expect(wrapper.text()).toContain('読み込み中')
    expect(wrapper.findComponent(ScoreCard).exists()).toBe(false)
  })
})
