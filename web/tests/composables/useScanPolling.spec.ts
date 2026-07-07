import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mockNuxtImport } from '@nuxt/test-utils/runtime'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

mockNuxtImport('useApiClient', () => () => ({ GET: get }))

const { useScanPolling } = await import('~/composables/useScanPolling')

describe('useScanPolling', () => {
  beforeEach(() => {
    get.mockReset()
    vi.useFakeTimers()
  })
  afterEach(() => vi.useRealTimers())

  it('running→done で停止し done=true になる', async () => {
    get
      .mockResolvedValueOnce({ data: { id: 'x', status: 'running' }, error: undefined })
      .mockResolvedValueOnce({ data: { id: 'x', status: 'done', summary: { score: 90 } }, error: undefined })
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0) // 初回即時取得
    expect(p.state.value?.status).toBe('running')
    expect(p.done.value).toBe(false)
    await vi.advanceTimersByTimeAsync(1000) // 2 回目
    expect(p.state.value?.status).toBe('done')
    expect(p.done.value).toBe(true)
    expect(get.mock.calls.length).toBe(2) // 停止後は呼ばれない
  })

  it('failed で done=true・状態は failed', async () => {
    get.mockResolvedValueOnce({ data: { id: 'x', status: 'failed' }, error: undefined })
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.done.value).toBe(true)
    expect(p.state.value?.status).toBe('failed')
  })

  it('取得エラーは error にセットし停止する', async () => {
    get.mockResolvedValueOnce({ data: undefined, error: { error: '見つかりません' } })
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.error.value).toContain('見つかりません')
    expect(p.done.value).toBe(true)
  })

  it('opts 省略時はデフォルト間隔（2500ms）でポーリングする', async () => {
    get
      .mockResolvedValueOnce({ data: { id: 'x', status: 'running' }, error: undefined })
      .mockResolvedValueOnce({ data: { id: 'x', status: 'done' }, error: undefined })
    const p = useScanPolling('x')
    p.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(get.mock.calls.length).toBe(1)
    await vi.advanceTimersByTimeAsync(2500)
    expect(get.mock.calls.length).toBe(2)
    expect(p.done.value).toBe(true)
  })

  it('stop を明示的に呼ぶとタイマーが解除され以後ポーリングしない', async () => {
    get.mockResolvedValue({ data: { id: 'x', status: 'running' }, error: undefined })
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0)
    p.stop()
    await vi.advanceTimersByTimeAsync(5000)
    expect(get.mock.calls.length).toBe(1)
  })

  it('in-flight 中に stop すると解決後も状態更新・再スケジュールしない', async () => {
    let resolveGet!: (v: { data: unknown; error: unknown }) => void
    get.mockImplementationOnce(
      () => new Promise((resolve) => { resolveGet = resolve }),
    )
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0) // GET は pending のまま
    p.stop() // 解決前に停止（unmount 相当）
    resolveGet({ data: { id: 'x', status: 'running' }, error: undefined })
    await vi.advanceTimersByTimeAsync(5000)
    expect(get.mock.calls.length).toBe(1) // 再スケジュールされない
    expect(p.state.value).toBeNull() // 停止後は状態を触らない
    expect(p.done.value).toBe(false)
  })

  it('start を再度呼ぶと既存タイマーを解除してから再開する（多重ポーリングしない）', async () => {
    get.mockResolvedValue({ data: { id: 'x', status: 'running' }, error: undefined })
    const p = useScanPolling('x', { intervalMs: 1000 })
    p.start()
    await vi.advanceTimersByTimeAsync(0) // 1 回目・旧 timer が arm される
    p.start() // 旧 timer を解除して再開
    await vi.advanceTimersByTimeAsync(0) // 2 回目（再開の即時 tick）
    await vi.advanceTimersByTimeAsync(1000) // 3 回目（単一チェーンのみ）
    expect(get.mock.calls.length).toBe(3)
  })

  it('コンポーネント内で使うと unmount 時に自動停止する', async () => {
    get.mockResolvedValue({ data: { id: 'x', status: 'running' }, error: undefined })
    let handle!: ReturnType<typeof useScanPolling>
    const Harness = defineComponent({
      setup() {
        handle = useScanPolling('x', { intervalMs: 1000 })
        handle.start()
        return () => h('div')
      },
    })
    const wrapper = mount(Harness)
    await vi.advanceTimersByTimeAsync(0)
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(5000)
    expect(get.mock.calls.length).toBe(1)
  })
})
