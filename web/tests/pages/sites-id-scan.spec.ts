import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import ScanWizard from '~/pages/sites/[id]/scan.vue'

const { postMock, navigateMock } = vi.hoisted(() => ({ postMock: vi.fn(), navigateMock: vi.fn() }))

mockNuxtImport('useApiClient', () => () => ({ POST: postMock }))
mockNuxtImport('navigateTo', () => navigateMock)

const ROUTE = '/sites/site-1/scan'

describe('pages/sites/[id]/scan', () => {
  beforeEach(() => {
    postMock.mockReset()
    navigateMock.mockReset()
  })

  it('危険パス除外の情報表示とプリセットを描画する', async () => {
    const w = await mountSuspended(ScanWizard, { route: ROUTE })
    expect(w.text()).toContain('自動で除外')
    expect(w.findAll('[data-preset-card]').length).toBe(3)
  })

  it('スキャン開始で POST /scans し結果ページへ遷移する', async () => {
    postMock.mockResolvedValueOnce({ data: { scan_id: 'scan-9', status: 'queued', preset: 'standard' }, error: undefined })
    const w = await mountSuspended(ScanWizard, { route: ROUTE })
    await w.find('[data-testid="start-scan"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(postMock).toHaveBeenCalledWith('/scans', { body: { site_id: 'site-1', preset: 'standard' } })
    expect(navigateMock).toHaveBeenCalledWith('/scans/scan-9')
  })

  it('開始エラーはメッセージを表示する', async () => {
    postMock.mockResolvedValueOnce({ data: undefined, error: { error: '所有確認が必要です' } })
    const w = await mountSuspended(ScanWizard, { route: ROUTE })
    await w.find('[data-testid="start-scan"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('所有確認が必要です')
  })

  it('プリセットを変更すると選択した preset で開始する', async () => {
    postMock.mockResolvedValueOnce({ data: { scan_id: 'scan-1', status: 'queued', preset: 'deep' }, error: undefined })
    const w = await mountSuspended(ScanWizard, { route: ROUTE })
    await w.find('[data-testid="preset-deep"]').trigger('click')
    await w.find('[data-testid="start-scan"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(postMock).toHaveBeenCalledWith('/scans', { body: { site_id: 'site-1', preset: 'deep' } })
  })
})
