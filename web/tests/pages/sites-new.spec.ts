import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import SitesNew from '~/pages/sites/new.vue'

const { postMock } = vi.hoisted(() => ({ postMock: vi.fn() }))

mockNuxtImport('useApiClient', () => () => ({ POST: postMock }))

describe('pages/sites/new', () => {
  beforeEach(() => postMock.mockReset())

  it('localhost 登録は即 verified で成功導線を表示する', async () => {
    postMock.mockResolvedValueOnce({
      data: { id: 'site-1', name: 'X', base_url: 'http://localhost:3001', ownership_verified: true },
      error: undefined,
    })
    const w = await mountSuspended(SitesNew)
    await w.find('[data-testid="field-name"]').setValue('X')
    await w.find('[data-testid="field-base-url"]').setValue('http://localhost:3001')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r))
    expect(postMock).toHaveBeenCalledWith('/sites', expect.objectContaining({ body: expect.objectContaining({ name: 'X' }) }))
    expect(w.text()).toContain('登録が完了しました')
    expect(w.find('[data-testid="go-site"]').exists()).toBe(true)
  })

  it('非ローカルは所有確認ガイドを表示し、確認成功で verified になる', async () => {
    postMock
      .mockResolvedValueOnce({
        data: {
          id: 'site-2',
          name: 'Y',
          base_url: 'https://example.com',
          ownership_verified: false,
          verification: { method: 'file', file_path: '/x', file_content: 'tok' },
        },
        error: undefined,
      })
      .mockResolvedValueOnce({ data: { id: 'site-2', ownership_verified: true }, error: undefined })
    const w = await mountSuspended(SitesNew)
    await w.find('[data-testid="field-name"]').setValue('Y')
    await w.find('[data-testid="field-base-url"]').setValue('https://example.com')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r))
    expect(w.find('[data-testid="guide-file"]').exists()).toBe(true)
    await w.find('[data-testid="verify"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('所有確認が完了しました')
  })

  it('登録エラーはメッセージを表示する', async () => {
    postMock.mockResolvedValueOnce({ data: undefined, error: { error: 'サイト名は既に使われています' } })
    const w = await mountSuspended(SitesNew)
    await w.find('[data-testid="field-name"]').setValue('dup')
    await w.find('[data-testid="field-base-url"]').setValue('http://localhost:3001')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('サイト名は既に使われています')
  })

  it('確認エラー（422）はメッセージを表示しガイドを維持する', async () => {
    postMock
      .mockResolvedValueOnce({
        data: {
          id: 'site-3',
          name: 'Z',
          base_url: 'https://example.com',
          ownership_verified: false,
          verification: { method: 'file', file_path: '/x', file_content: 'tok' },
        },
        error: undefined,
      })
      .mockResolvedValueOnce({ data: undefined, error: { error: 'ファイルの設置が確認できませんでした' } })
    const w = await mountSuspended(SitesNew)
    await w.find('[data-testid="field-name"]').setValue('Z')
    await w.find('[data-testid="field-base-url"]').setValue('https://example.com')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r))
    await w.find('[data-testid="verify"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(w.text()).toContain('ファイルの設置が確認できませんでした')
    expect(w.find('[data-testid="guide-file"]').exists()).toBe(true)
  })

  it('サイト ID が欠損した応答では確認処理を行わない', async () => {
    postMock.mockResolvedValueOnce({
      data: {
        name: 'W',
        base_url: 'https://example.com',
        ownership_verified: false,
        verification: { method: 'file', file_path: '/x', file_content: 'tok' },
      },
      error: undefined,
    })
    const w = await mountSuspended(SitesNew)
    await w.find('[data-testid="field-name"]').setValue('W')
    await w.find('[data-testid="field-base-url"]').setValue('https://example.com')
    await w.find('form').trigger('submit.prevent')
    await new Promise((r) => setTimeout(r))
    await w.find('[data-testid="verify"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(postMock).toHaveBeenCalledTimes(1)
    expect(w.find('[data-testid="guide-file"]').exists()).toBe(true)
  })
})
