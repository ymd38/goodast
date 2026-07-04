import { describe, expect, it } from 'vitest'
import { formatDelta, scoreBandStyle } from '~/utils/score-band'

describe('scoreBandStyle', () => {
  it.each([
    ['good', 'text-success', false],
    ['caution', 'text-warning', false],
    ['danger', 'text-m-red', false],
    ['crisis', 'text-m-red', true],
    ['unknown-band', 'text-muted', false],
    [undefined, 'text-muted', false],
  ])('band=%s → text=%s emphasis=%s', (band, text, emphasis) => {
    expect(scoreBandStyle(band)).toEqual({ text, emphasis })
  })
})

describe('formatDelta', () => {
  it.each([
    [5, '+5↑'],
    [-12, '-12↓'],
    [0, '±0'],
    [null, null],
    [undefined, null],
  ])('delta=%s → %s', (delta, expected) => {
    expect(formatDelta(delta)).toBe(expected)
  })
})
