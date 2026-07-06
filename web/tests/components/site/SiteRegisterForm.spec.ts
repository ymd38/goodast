import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SiteRegisterForm from '~/components/site/SiteRegisterForm.vue'

describe('SiteRegisterForm', () => {
  it('入力して submit すると payload を emit する', async () => {
    const w = mount(SiteRegisterForm, { props: { submitting: false, error: null } })
    await w.find('[data-testid="field-name"]').setValue('My Site')
    await w.find('[data-testid="field-base-url"]').setValue('http://localhost:3001')
    await w.find('form').trigger('submit.prevent')
    const emitted = w.emitted('submit')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({ name: 'My Site', base_url: 'http://localhost:3001', verify_method: 'file' })
  })

  it('submitting 中は送信ボタンを無効化する', () => {
    const w = mount(SiteRegisterForm, { props: { submitting: true, error: null } })
    expect(w.find('[data-testid="submit"]').attributes('disabled')).toBeDefined()
  })

  it('error があれば表示する', () => {
    const w = mount(SiteRegisterForm, { props: { submitting: false, error: '登録に失敗しました' } })
    expect(w.text()).toContain('登録に失敗しました')
  })
})
