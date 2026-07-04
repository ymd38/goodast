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
