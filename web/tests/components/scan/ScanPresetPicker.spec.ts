import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ScanPresetPicker from '~/components/scan/ScanPresetPicker.vue'

describe('ScanPresetPicker', () => {
  it('3 プリセットを描画し、選択中を強調する', () => {
    const w = mount(ScanPresetPicker, { props: { modelValue: 'standard' } })
    const cards = w.findAll('[data-preset-card]')
    expect(cards).toHaveLength(3)
    expect(w.find('[data-testid="preset-standard"]').classes()).toContain('border-on-dark')
  })
  it('カードクリックで update:modelValue を emit する', async () => {
    const w = mount(ScanPresetPicker, { props: { modelValue: 'standard' } })
    await w.find('[data-testid="preset-deep"]').trigger('click')
    expect(w.emitted('update:modelValue')![0][0]).toBe('deep')
  })
})
