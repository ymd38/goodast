export interface ScoreBandStyle {
  /** スコア数字に付けるテキスト色クラス */
  text: string
  /** crisis の opacity 強調（frontend.md「セキュリティスコアの色分け」） */
  emphasis: boolean
}

// backend は Band（セマンティック）を返し、tokens.css の CSS 変数へのマップは
// frontend の責務（PROGRESS.md の責務分離決定）。未知 Band は muted に落とす（前方互換）
export function scoreBandStyle(band: string | undefined): ScoreBandStyle {
  switch (band) {
    case 'good':
      return { text: 'text-success', emphasis: false }
    case 'caution':
      return { text: 'text-warning', emphasis: false }
    case 'danger':
      return { text: 'text-m-red', emphasis: false }
    case 'crisis':
      return { text: 'text-m-red', emphasis: true }
    default:
      return { text: 'text-muted', emphasis: false }
  }
}

// 前回差分の表示形式は企画書 §6-6（例: +5↑ / -12↓）
export function formatDelta(delta: number | null | undefined): string | null {
  if (delta == null) return null
  if (delta > 0) return `+${delta}↑`
  if (delta < 0) return `${delta}↓`
  return '±0'
}
