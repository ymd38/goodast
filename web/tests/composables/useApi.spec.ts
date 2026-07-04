import { afterEach, describe, expect, it, vi } from 'vitest'

describe('useApiClient', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('runtimeConfig の apiBase を前置して fetch する', async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const client = useApiClient()
    const { data, error } = await client.GET('/sites')

    expect(error).toBeUndefined()
    expect(data).toEqual([])
    expect(fetchMock).toHaveBeenCalledTimes(1)
    // openapi-fetch は Request オブジェクトを構築して fetch(request) を呼ぶため、この cast は実行時に安全
    const request = fetchMock.mock.calls[0][0] as Request
    expect(request.url).toContain('/api/sites')
  })
})
