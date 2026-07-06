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
