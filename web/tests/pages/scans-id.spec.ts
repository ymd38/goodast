import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import { ref } from 'vue'
import type { ScanState } from '~/types/goodast'
import ScanResultPage from '~/pages/scans/[id].vue'

const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }))

const polling = {
  state: ref<ScanState | null>(null),
  error: ref<string | null>(null),
  done: ref(false),
  start: vi.fn(),
  stop: vi.fn(),
}

mockNuxtImport('useApiClient', () => () => ({ GET: getMock }))
mockNuxtImport('useScanPolling', () => () => polling)

const ROUTE = '/scans/scan-1'

// polling は mockNuxtImport 経由の共有シングルトンのため、前テストの watch が
// 後続テストの状態変更で誤発火しないよう、都度 unmount してから次のテストへ進む
let activeWrapper: Awaited<ReturnType<typeof mountSuspended>> | undefined

async function mountPage() {
  activeWrapper = await mountSuspended(ScanResultPage, { route: ROUTE })
  return activeWrapper
}

describe('pages/scans/[id]', () => {
  beforeEach(() => {
    getMock.mockReset()
    polling.start.mockReset()
    polling.stop.mockReset()
    polling.state.value = null
    polling.done.value = false
    polling.error.value = null
  })

  afterEach(() => {
    activeWrapper?.unmount()
    activeWrapper = undefined
  })

  it('マウントでポーリングを開始し、進捗中は ScanProgress を表示', async () => {
    polling.state.value = { status: 'running' }
    const w = await mountPage()
    expect(polling.start).toHaveBeenCalled()
    expect(w.text()).toContain('スキャン実行中')
  })

  it('state 未取得（初回ポーリング前）は待機中表示になる', async () => {
    const w = await mountPage()
    expect(w.text()).toContain('待機中')
    expect(getMock).not.toHaveBeenCalled()
  })

  it('ポーリング自体が失敗した場合はエラーを表示し進捗表示に留まらない', async () => {
    polling.done.value = true
    polling.error.value = 'スキャンが見つかりません'
    // state は null のまま（poll 自体が最初から失敗して停止したケース）
    const w = await mountPage()
    expect(w.find('[data-testid="poll-error"]').text()).toContain('スキャンが見つかりません')
    expect(w.text()).not.toContain('待機中')
    expect(getMock).not.toHaveBeenCalled()
  })

  it('done で findings を取得して結果を表示する', async () => {
    polling.state.value = {
      id: 'scan-1',
      status: 'done',
      summary: { score: 90, band: 'good', label: '良好', counts: {} },
    }
    polling.done.value = true
    getMock.mockResolvedValueOnce({
      data: {
        findings: [
          { id: 'f1', title: 'XSS', severity: 'High', url: 'u', cwe: '', remediation: '', template_id: 't', status: 'open' },
        ],
      },
      error: undefined,
    })
    const w = await mountPage()
    await new Promise((r) => setTimeout(r))
    expect(getMock).toHaveBeenCalledWith('/scans/{id}/findings', { params: { path: { id: 'scan-1' } } })
    expect(w.text()).toContain('XSS')
    expect(w.text()).toContain('専門家')
  })

  it('findings が空（0件）の場合は「検出はありませんでした」を表示する', async () => {
    polling.state.value = {
      id: 'scan-1',
      status: 'done',
      summary: { score: 100, band: 'good', label: '良好', counts: {} },
    }
    polling.done.value = true
    getMock.mockResolvedValueOnce({ data: {}, error: undefined })
    const w = await mountPage()
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('検出はありませんでした')
  })

  it('findings 取得エラーはエラーメッセージを表示する', async () => {
    polling.state.value = {
      id: 'scan-1',
      status: 'done',
      summary: { score: 50, band: 'danger', label: '危険', counts: {} },
    }
    polling.done.value = true
    getMock.mockResolvedValueOnce({ data: undefined, error: { error: '取得に失敗しました' } })
    const w = await mountPage()
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('取得に失敗しました')
  })

  it('findings がエラーなく未取得（data undefined）の場合もフォールバックメッセージを表示する', async () => {
    polling.state.value = {
      id: 'scan-1',
      status: 'done',
      summary: { score: 50, band: 'danger', label: '危険', counts: {} },
    }
    polling.done.value = true
    getMock.mockResolvedValueOnce({ data: undefined, error: undefined })
    const w = await mountPage()
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('APIとの通信に失敗しました')
  })

  it('done だが summary 未取得の場合はスコアカードが未実行表示になる', async () => {
    polling.state.value = { id: 'scan-1', status: 'done' }
    polling.done.value = true
    getMock.mockResolvedValueOnce({ data: { findings: [] }, error: undefined })
    const w = await mountPage()
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('スキャン未実行')
  })

  it('failed は失敗表示', async () => {
    polling.state.value = { status: 'failed' }
    polling.done.value = true
    const w = await mountPage()
    expect(w.find('[data-testid="scan-failed"]').exists()).toBe(true)
  })
})
