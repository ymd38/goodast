package report

import "time"

// ScanPoint は完了 1 スキャンのダッシュボード入力（スコア算出前）。repository が
// scans 行から組み立てる。Date は完了時刻（無ければ作成時刻）を採用する。
type ScanPoint struct {
	ScanID string
	Date   time.Time
	Counts SeverityCounts
}

// HistoryEntry はスコア時系列の 1 点（中段の折れ線・下段の積み上げ棒の描画用）。
type HistoryEntry struct {
	ScanID string         `json:"scan_id"`
	Date   time.Time      `json:"date"`
	Score  int            `json:"score"`
	Band   Band           `json:"band"`
	Counts SeverityCounts `json:"counts"`
}

// LatestState は上段「状態（今どうか）」のデータ。Delta は前回スキャンとの差分で、
// 前回が無い（初回スキャン）場合は nil。
type LatestState struct {
	ScanID string         `json:"scan_id"`
	Date   time.Time      `json:"date"`
	Score  int            `json:"score"`
	Band   Band           `json:"band"`
	Label  string         `json:"label"`
	Delta  *int           `json:"delta"`
	Counts SeverityCounts `json:"counts"`
}

// DashboardData はサイト 1 件のダッシュボード集計結果。
// Latest は done スキャンが 1 件も無ければ nil。History は日付昇順（折れ線 左→右）で、
// 常に非 nil（空スライス）。
type DashboardData struct {
	Latest  *LatestState   `json:"latest"`
	History []HistoryEntry `json:"history"`
}

// BuildDashboard は日付昇順の完了スキャン列から、状態（最新スコア＋前回差分）と
// 遷移（スコア時系列）を集計する純粋関数。スコアは各点の重大度別件数から算出する。
func BuildDashboard(points []ScanPoint) DashboardData {
	history := make([]HistoryEntry, 0, len(points))
	scores := make([]Score, 0, len(points))
	for _, p := range points {
		s := Compute(p.Counts)
		scores = append(scores, s)
		history = append(history, HistoryEntry{
			ScanID: p.ScanID,
			Date:   p.Date,
			Score:  s.Value(),
			Band:   s.Band(),
			Counts: p.Counts,
		})
	}
	if len(history) == 0 {
		return DashboardData{History: history}
	}

	last := len(history) - 1
	latest := scores[last]
	var delta *int
	if last >= 1 {
		d := latest.Delta(scores[last-1])
		delta = &d
	}
	lp := points[last]
	return DashboardData{
		Latest: &LatestState{
			ScanID: lp.ScanID,
			Date:   lp.Date,
			Score:  latest.Value(),
			Band:   latest.Band(),
			Label:  latest.Label(),
			Delta:  delta,
			Counts: lp.Counts,
		},
		History: history,
	}
}
