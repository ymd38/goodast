package engine

import "github.com/ymd38/goodast/jobs"

// Summarize は findings を重大度別に集計する純粋関数。集計結果は scans.summary_json の
// 共有 wire 型 jobs.SeverityCounts で返し、api（スコア計算）と形を共有する（ドリフト防止）。
func Summarize(findings []Finding) jobs.SeverityCounts {
	var s jobs.SeverityCounts
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			s.Critical++
		case SeverityHigh:
			s.High++
		case SeverityMedium:
			s.Medium++
		case SeverityLow:
			s.Low++
		case SeverityInfo:
			s.Info++
		}
	}
	s.Total = len(findings)
	return s
}
