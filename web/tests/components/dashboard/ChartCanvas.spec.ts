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
