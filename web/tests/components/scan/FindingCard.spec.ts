import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FindingCard from '~/components/scan/FindingCard.vue'
import SeverityBadge from '~/components/scan/SeverityBadge.vue'

const finding = { id: 'f1', template_id: 't1', title: 'XSS', severity: 'High', url: 'http://x/y', cwe: 'CWE-79', remediation: 'エスケープする', status: 'open' }

describe('FindingCard', () => {
  it('タイトル・URL・CWE・修正方法を表示する', () => {
    const w = mount(FindingCard, { props: { finding }, global: { components: { SeverityBadge } } })
    expect(w.text()).toContain('XSS')
    expect(w.text()).toContain('http://x/y')
    expect(w.text()).toContain('CWE-79')
    expect(w.text()).toContain('エスケープする')
  })
  it('cwe / remediation が空なら該当行を出さない', () => {
    const w = mount(FindingCard, { props: { finding: { ...finding, cwe: '', remediation: '' } }, global: { components: { SeverityBadge } } })
    expect(w.find('[data-testid="finding-cwe"]').exists()).toBe(false)
    expect(w.find('[data-testid="finding-remediation"]').exists()).toBe(false)
  })
})
