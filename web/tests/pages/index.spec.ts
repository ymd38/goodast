import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import type { Site } from '~/types/goodast'
import IndexPage from '~/pages/index.vue'

const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }))

mockNuxtImport('useApiClient', () => () => ({ GET: getMock }))

const sites: Site[] = [
  {
    id: '11111111-1111-1111-1111-111111111111',
    name: 'コーポレートサイト',
    base_url: 'https://example.com',
    ownership_verified: true,
  },
  {
    id: '22222222-2222-2222-2222-222222222222',
    name: 'ローカル検証',
    base_url: 'http://localhost:3001',
    ownership_verified: false,
  },
]

describe('pages/index', () => {
  beforeEach(() => {
    getMock.mockReset()
  })

  it('サイトカードを一覧描画しダッシュボードへリンクする', async () => {
    getMock.mockResolvedValue({ data: sites, error: undefined })
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain('コーポレートサイト')
    expect(wrapper.text()).toContain('https://example.com')
    expect(wrapper.text()).toContain('所有確認済み')
    expect(wrapper.text()).toContain('所有未確認')
    expect(
      wrapper.find('a[href="/sites/11111111-1111-1111-1111-111111111111"]').exists(),
    ).toBe(true)
    // 所有確認バッジの色クラスを assert する
    const badges = wrapper.findAll('[data-testid="ownership-badge"]')
    expect(badges).toHaveLength(2)
    expect(badges[0]!.classes()).toContain('text-success')
    expect(badges[1]!.classes()).toContain('text-warning')
  })

  it('0 件なら未登録の空状態を表示する', async () => {
    getMock.mockResolvedValue({ data: [], error: undefined })
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain(
      'サイトが未登録です。右上の『サイトを登録』から追加できます。',
    )
  })

  it('サイト登録への導線を表示する', async () => {
    getMock.mockResolvedValue({ data: [], error: undefined })
    const wrapper = await mountSuspended(IndexPage)
    const link = wrapper.find('[data-testid="register-link"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toBe('/sites/new')
  })

  it('data が undefined でも空状態にフォールバックする', async () => {
    getMock.mockResolvedValue({ data: undefined, error: undefined })
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain('サイトが未登録です')
  })

  it('API エラーはエラーバンドを表示する', async () => {
    getMock.mockResolvedValue({ data: undefined, error: { error: 'boom' } })
    const wrapper = await mountSuspended(IndexPage)
    expect(wrapper.text()).toContain('boom')
  })
})
