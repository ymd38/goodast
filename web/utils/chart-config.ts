import type { ChartConfiguration } from 'chart.js'
import type { HistoryEntry } from '~/types/goodast'

/** 遷移グラフの描画に必要な最小スキャン数（web/CLAUDE.md: 2 未満は「データ不足」） */
export const MIN_TREND_POINTS = 2

/** Chart.js へ注入する色。canvas は CSS 変数を継承しないため実値を渡す（テストでは任意文字列で注入） */
export interface ChartPalette {
  line: string
  grid: string
  text: string
  severity: {
    critical: string
    high: string
    medium: string
    low: string
    info: string
  }
}

export function hasTrendData(history: readonly HistoryEntry[]): boolean {
  return history.length >= MIN_TREND_POINTS
}

export function buildScoreTrendConfig(
  history: readonly HistoryEntry[],
  palette: ChartPalette,
): ChartConfiguration<'line'> {
  return {
    type: 'line',
    data: {
      labels: history.map((h) => h.date ?? ''),
      datasets: [
        {
          label: 'Goodast Security Score',
          data: history.map((h) => h.score ?? 0),
          borderColor: palette.line,
          backgroundColor: palette.line,
          tension: 0.2,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        y: {
          min: 0,
          max: 100,
          grid: { color: palette.grid },
          ticks: { color: palette.text },
        },
        x: {
          grid: { color: palette.grid },
          ticks: { color: palette.text },
        },
      },
      plugins: { legend: { display: false } },
    },
  }
}

const SEVERITY_KEYS = ['critical', 'high', 'medium', 'low', 'info'] as const

const SEVERITY_LABELS: Record<(typeof SEVERITY_KEYS)[number], string> = {
  critical: 'Critical',
  high: 'High',
  medium: 'Medium',
  low: 'Low',
  info: 'Info',
}

export function buildSeverityStackConfig(
  history: readonly HistoryEntry[],
  palette: ChartPalette,
): ChartConfiguration<'bar'> {
  return {
    type: 'bar',
    data: {
      labels: history.map((h) => h.date ?? ''),
      datasets: SEVERITY_KEYS.map((key) => ({
        label: SEVERITY_LABELS[key],
        data: history.map((h) => h.counts?.[key] ?? 0),
        backgroundColor: palette.severity[key],
        stack: 'findings',
      })),
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { stacked: true, grid: { color: palette.grid }, ticks: { color: palette.text } },
        y: { stacked: true, grid: { color: palette.grid }, ticks: { color: palette.text } },
      },
      plugins: { legend: { labels: { color: palette.text } } },
    },
  }
}
