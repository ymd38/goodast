import { afterEach, describe, expect, it } from 'vitest'
import { resolveChartPalette } from '~/utils/chart-palette'

const TOKENS = [
  '--color-m-blue-dark',
  '--color-hairline',
  '--color-body',
  '--color-m-red',
  '--color-warning',
  '--color-m-blue-light',
  '--color-muted',
] as const

describe('resolveChartPalette', () => {
  afterEach(() => {
    for (const t of TOKENS) document.documentElement.style.removeProperty(t)
  })

  it('documentElement の CSS 変数から役割別の色を解決する（trim 込み)', () => {
    const root = document.documentElement
    root.style.setProperty('--color-m-blue-dark', ' rgb(28, 105, 212) ')
    root.style.setProperty('--color-hairline', 'rgb(60, 60, 60)')
    root.style.setProperty('--color-body', 'rgb(187, 187, 187)')
    root.style.setProperty('--color-m-red', 'rgb(226, 39, 24)')
    root.style.setProperty('--color-warning', 'rgb(244, 180, 0)')
    root.style.setProperty('--color-m-blue-light', 'rgb(0, 102, 177)')
    root.style.setProperty('--color-muted', 'rgb(126, 126, 126)')

    expect(resolveChartPalette()).toEqual({
      line: 'rgb(28, 105, 212)',
      grid: 'rgb(60, 60, 60)',
      text: 'rgb(187, 187, 187)',
      severity: {
        critical: 'rgb(226, 39, 24)',
        high: 'rgb(244, 180, 0)',
        medium: 'rgb(28, 105, 212)',
        low: 'rgb(0, 102, 177)',
        info: 'rgb(126, 126, 126)',
      },
    })
  })
})
