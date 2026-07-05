import { describe, expect, it } from 'vitest'
import { toApiErrorMessage } from '~/composables/useApiError'

describe('toApiErrorMessage', () => {
  it.each([
    [{ error: 'site not found' }, 'site not found'],
    [{ error: '' }, 'APIとの通信に失敗しました。時間をおいて再度お試しください。'],
    [{ message: 'not-api-shape' }, 'APIとの通信に失敗しました。時間をおいて再度お試しください。'],
    [null, 'APIとの通信に失敗しました。時間をおいて再度お試しください。'],
    ['plain string', 'APIとの通信に失敗しました。時間をおいて再度お試しください。'],
  ])('%j → %s', (input, expected) => {
    expect(toApiErrorMessage(input)).toBe(expected)
  })
})
