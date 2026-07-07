import { describe, expect, it } from 'vitest'
import { SCAN_PRESETS, DEFAULT_PRESET } from '~/utils/scan-preset'

describe('scan-preset', () => {
  it('3 プリセットを backend の値で定義する', () => {
    expect(SCAN_PRESETS.map((p) => p.value)).toEqual(['light', 'standard', 'deep'])
    for (const p of SCAN_PRESETS) {
      expect(p.label).toBeTruthy()
      expect(p.description).toBeTruthy()
      expect(p.estimate).toBeTruthy()
    }
  })
  it('既定は standard', () => {
    expect(DEFAULT_PRESET).toBe('standard')
    expect(SCAN_PRESETS.some((p) => p.value === DEFAULT_PRESET)).toBe(true)
  })
})
