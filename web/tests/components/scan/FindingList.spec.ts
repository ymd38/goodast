import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import FindingList from '~/components/scan/FindingList.vue'
import FindingCard from '~/components/scan/FindingCard.vue'
import SeverityBadge from '~/components/scan/SeverityBadge.vue'

const mk = (severity: string, id: string): import('~/types/goodast').Finding => ({ id, template_id: id, title: id, severity, url: 'u', cwe: '', remediation: '', status: 'open' })

describe('FindingList', () => {
  it('重大度順に並べて描画する', () => {
    const w = mount(FindingList, { props: { findings: [mk('Low', 'a'), mk('Critical', 'b')] }, global: { components: { ScanFindingCard: FindingCard, ScanSeverityBadge: SeverityBadge } } })
    const cards = w.findAll('[data-testid="finding"]')
    expect(cards).toHaveLength(2)
    expect(cards[0].text()).toContain('b') // Critical 先頭
  })
  it('Nuxt auto-import で ScanFindingCard/ScanSeverityBadge が実解決される', async () => {
    // 手動登録（global.components）はプレフィックス誤りを隠すため、実 auto-import 経路で入れ子解決を証明する
    const w = await mountSuspended(FindingList, { props: { findings: [mk('Critical', 'x')] } })
    expect(w.find('[data-testid="finding"]').exists()).toBe(true)
    expect(w.find('[data-testid="severity-badge"]').text()).toContain('Critical')
  })
  it('0 件は「検出はありませんでした」を表示', () => {
    const w = mount(FindingList, { props: { findings: [] }, global: { components: { ScanFindingCard: FindingCard, ScanSeverityBadge: SeverityBadge } } })
    expect(w.text()).toContain('検出はありませんでした')
    expect(w.find('[data-testid="finding"]').exists()).toBe(false)
  })
})
