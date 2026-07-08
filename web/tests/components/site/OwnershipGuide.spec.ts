import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import OwnershipGuide from '~/components/site/OwnershipGuide.vue'

describe('OwnershipGuide', () => {
  it('file 方式はファイルパスと内容の手順を表示する', () => {
    const w = mount(OwnershipGuide, {
      props: { verification: { method: 'file', file_path: '/.well-known/goodast.txt', file_content: 'token-abc' } },
    })
    expect(w.text()).toContain('/.well-known/goodast.txt')
    expect(w.text()).toContain('token-abc')
    expect(w.find('[data-testid="guide-file"]').exists()).toBe(true)
    expect(w.find('[data-testid="guide-dns"]').exists()).toBe(false)
  })

  it('dns 方式は TXT レコードの手順を表示する', () => {
    const w = mount(OwnershipGuide, {
      props: { verification: { method: 'dns', dns_record: 'goodast-verify=token-abc' } },
    })
    expect(w.text()).toContain('goodast-verify=token-abc')
    expect(w.find('[data-testid="guide-dns"]').exists()).toBe(true)
    expect(w.find('[data-testid="guide-file"]').exists()).toBe(false)
  })
})
