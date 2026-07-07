export type ScanPresetValue = 'light' | 'standard' | 'deep'

export interface ScanPresetOption {
  value: ScanPresetValue
  label: string
  description: string
  estimate: string
}

// backend jobs.Preset と一致（軽量/標準/詳細）。所要目安は engine.PlanFor のタイムアウト上限に対応。
export const SCAN_PRESETS: ScanPresetOption[] = [
  { value: 'light', label: '軽量', description: '基本的な設定ミス・技術検出のみ。素早く確認したいとき。', estimate: '目安 5 分以内' },
  { value: 'standard', label: '標準', description: '公開パネル・既定ログイン・CVE を含むバランス設定（推奨）。', estimate: '目安 15 分以内' },
  { value: 'deep', label: '詳細', description: 'XSS/SQLi/SSRF 等まで広く検査。時間をかけて網羅したいとき。', estimate: '目安 30 分以内' },
]

export const DEFAULT_PRESET: ScanPresetValue = 'standard'
