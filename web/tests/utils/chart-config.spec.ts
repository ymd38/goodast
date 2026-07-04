import { describe, expect, it } from 'vitest'
import type { HistoryEntry } from '~/types/goodast'
import {
  MIN_TREND_POINTS,
  buildScoreTrendConfig,
  buildSeverityStackConfig,
  hasTrendData,
  type ChartPalette,
} from '~/utils/chart-config'

const palette: ChartPalette = {
  line: 'line-color',
  grid: 'grid-color',
  text: 'text-color',
  severity: {
    critical: 'c-color',
    high: 'h-color',
    medium: 'm-color',
    low: 'l-color',
    info: 'i-color',
  },
}

const fullEntry: HistoryEntry = {
  scan_id: 's-1',
  date: '2026-07-01',
  score: 67,
  band: 'caution',
  counts: { critical: 1, high: 2, medium: 3, low: 4, info: 5, total: 15 },
}

// 生成型は全フィールド optional（swagger 2.0 に required 指定がないため）。
// 欠損時のフォールバック分岐を空オブジェクトで検証する
const emptyEntry: HistoryEntry = {}

describe('hasTrendData', () => {
  it.each([
    [0, false],
    [1, false],
    [2, true],
  ])('history %d 件 → %s', (n, expected) => {
    expect(hasTrendData(Array.from({ length: n }, () => fullEntry))).toBe(expected)
    expect(MIN_TREND_POINTS).toBe(2)
  })
})

describe('buildScoreTrendConfig', () => {
  it('日付ラベルとスコア系列を palette の色で組み立てる', () => {
    const config = buildScoreTrendConfig([fullEntry, { ...fullEntry, date: '2026-07-02', score: 80 }], palette)
    expect(config.type).toBe('line')
    expect(config.data.labels).toEqual(['2026-07-01', '2026-07-02'])
    expect(config.data.datasets[0]!.data).toEqual([67, 80])
    expect(config.data.datasets[0]!.borderColor).toBe('line-color')
    expect(config.options?.scales?.y).toMatchObject({ min: 0, max: 100 })
  })

  it('date/score 欠損は空ラベル・0 にフォールバックする', () => {
    const config = buildScoreTrendConfig([emptyEntry], palette)
    expect(config.data.labels).toEqual([''])
    expect(config.data.datasets[0]!.data).toEqual([0])
  })
})

describe('buildSeverityStackConfig', () => {
  it('重大度 5 系列を stacked bar として palette の色で組み立てる', () => {
    const config = buildSeverityStackConfig([fullEntry], palette)
    expect(config.type).toBe('bar')
    expect(config.data.datasets).toHaveLength(5)
    expect(config.data.datasets.map((d) => d.label)).toEqual(['Critical', 'High', 'Medium', 'Low', 'Info'])
    expect(config.data.datasets.map((d) => d.data[0])).toEqual([1, 2, 3, 4, 5])
    expect(config.data.datasets[0]!.backgroundColor).toBe('c-color')
    expect(config.options?.scales?.x).toMatchObject({ stacked: true })
    expect(config.options?.scales?.y).toMatchObject({ stacked: true })
  })

  it('counts 欠損は全系列 0 にフォールバックする', () => {
    const config = buildSeverityStackConfig([emptyEntry], palette)
    expect(config.data.datasets.map((d) => d.data[0])).toEqual([0, 0, 0, 0, 0])
  })
})
