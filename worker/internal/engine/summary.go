package engine

// Summary は 1 スキャンの重大度別集計（scans.summary_json のダッシュボード描画用データ）。
// スコア計算は api 側 report に集約するため、ここでは件数の集計のみを持つ（責務分離）。
type Summary struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

// Summarize は findings を重大度別に集計する純粋関数。
func Summarize(findings []Finding) Summary {
	var s Summary
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
