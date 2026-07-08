import { describe, expect, it } from 'vitest'
import { severityTextClass, SEVERITY_ORDER, sortFindingsBySeverity } from '~/utils/severity'
import type { Finding } from '~/types/goodast'

describe('severity', () => {
  it('重大度をトークンクラスにマップする', () => {
    expect(severityTextClass('Critical')).toBe('text-m-red')
    expect(severityTextClass('High')).toBe('text-m-red')
    expect(severityTextClass('Medium')).toBe('text-warning')
    expect(severityTextClass('Low')).toBe('text-muted')
    expect(severityTextClass('Info')).toBe('text-muted')
    expect(severityTextClass('???')).toBe('text-muted')
  })
  it('重大度順（Critical→Info）に並べ替える', () => {
    const input = [
      { severity: 'Low' },
      { severity: 'Critical' },
      { severity: 'Medium' },
    ] as Pick<Finding, 'severity'>[]
    expect(sortFindingsBySeverity(input as Finding[]).map((f) => f.severity)).toEqual(['Critical', 'Medium', 'Low'])
    expect(SEVERITY_ORDER[0]).toBe('Critical')
  })
  it('未知の重大度を末尾にソートする', () => {
    const input = [
      { severity: 'Unknown' },
      { severity: 'Critical' },
      { severity: 'Low' },
    ] as Pick<Finding, 'severity'>[]
    expect(sortFindingsBySeverity(input as Finding[]).map((f) => f.severity)).toEqual(['Critical', 'Low', 'Unknown'])
  })
  it('severity 未設定（optional）も未知扱いで末尾にソートする', () => {
    const input = [{}, { severity: 'Low' }] as Finding[]
    expect(sortFindingsBySeverity(input).map((f) => f.severity)).toEqual(['Low', undefined])
  })
})
