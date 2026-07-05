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
