package engine

import "strings"

// Severity は findings テーブルの CHECK 制約に一致する正規化済み重大度。
// 値の集合は migrations/000001 の findings.severity CHECK と一致させること。
type Severity string

// 正規の重大度値（DB CHECK 制約と一致）。
const (
	SeverityCritical Severity = "Critical"
	SeverityHigh     Severity = "High"
	SeverityMedium   Severity = "Medium"
	SeverityLow      Severity = "Low"
	SeverityInfo     Severity = "Info"
)

// ParseSeverity は Nuclei テンプレートの severity 文字列を正規の Severity に変換する。
// Nuclei は info/low/medium/high/critical/unknown 等を返す。未知・空文字は安全側として
// Info に倒し、DB の CHECK 制約違反（＝finding の保存失敗）を防ぐ。
func ParseSeverity(s string) Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium":
		return SeverityMedium
	case "low":
		return SeverityLow
	case "info":
		return SeverityInfo
	default:
		return SeverityInfo
	}
}
