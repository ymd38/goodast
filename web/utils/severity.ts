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
  // 生成型の severity は optional（swagger 由来）。未設定は未知扱いで末尾に回す
  const rank = (s: string | undefined) => {
    const i = SEVERITY_ORDER.indexOf(s ?? '')
    return i === -1 ? SEVERITY_ORDER.length : i
  }
  return [...findings].sort((a, b) => rank(a.severity) - rank(b.severity))
}
