import { describe, expect, it } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import DefaultLayout from '~/layouts/default.vue'

describe('layouts/default', () => {
  it('ワードマーク・M ストライプ・slot 内容を描画する', async () => {
    const wrapper = await mountSuspended(DefaultLayout, {
      slots: { default: () => 'SLOT-CONTENT' },
    })
    expect(wrapper.text()).toContain('Goodast')
    expect(wrapper.text()).toContain('SLOT-CONTENT')
    expect(wrapper.find('.m-stripe').exists()).toBe(true)
    expect(wrapper.find('a[href="/"]').exists()).toBe(true)
  })
})
