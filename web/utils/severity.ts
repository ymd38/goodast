import type { Finding } from '~/types/goodast'

// 重大度→tokens.css セマンティッククラス（frontend.md「重大度カラー」）。未知は muted。
export function severityTextClass(sev: string): string {
  switch (sev) {
    case 'Critical':
    case 'High':
      return 'text-m-red'
    case 'Medium':
      return 'text-warning'
    default:
      return 'text-muted'
  }
}

export const SEVERITY_ORDER = ['Critical', 'High', 'Medium', 'Low', 'Info']

export function sortFindingsBySeverity(findings: Finding[]): Finding[] {
  const rank = (s: string) => {
    const i = SEVERITY_ORDER.indexOf(s)
    return i === -1 ? SEVERITY_ORDER.length : i
  }
  return [...findings].sort((a, b) => rank(a.severity) - rank(b.severity))
}
