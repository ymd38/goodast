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
