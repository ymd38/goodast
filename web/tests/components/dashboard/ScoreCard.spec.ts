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
