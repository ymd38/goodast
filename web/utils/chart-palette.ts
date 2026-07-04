import type { ChartPalette } from '~/utils/chart-config'

// canvas 描画は CSS 変数を継承しないため、tokens.css の値を実行時に解決して注入する。
// client 専用（<ClientOnly> 配下でのみ呼ぶこと）
export function resolveChartPalette(): ChartPalette {
  const styles = getComputedStyle(document.documentElement)
  const token = (name: string) => styles.getPropertyValue(name).trim()
  return {
    line: token('--color-m-blue-dark'),
    grid: token('--color-hairline'),
    text: token('--color-body'),
    severity: {
      critical: token('--color-m-red'),
      high: token('--color-warning'),
      medium: token('--color-m-blue-dark'),
      low: token('--color-m-blue-light'),
      info: token('--color-muted'),
    },
  }
}
