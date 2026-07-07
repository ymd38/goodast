import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ScanProgress from '~/components/scan/ScanProgress.vue'

describe('ScanProgress', () => {
  it('running は実行中表示', () => {
    const w = mount(ScanProgress, { props: { status: 'running' } })
    expect(w.text()).toContain('スキャン実行中')
  })
  it('queued は待機中表示', () => {
    const w = mount(ScanProgress, { props: { status: 'queued' } })
    expect(w.text()).toContain('待機中')
  })
  it('failed はエラー表示', () => {
    const w = mount(ScanProgress, { props: { status: 'failed' } })
    expect(w.find('[data-testid="scan-failed"]').exists()).toBe(true)
    expect(w.text()).toContain('失敗')
  })
})
