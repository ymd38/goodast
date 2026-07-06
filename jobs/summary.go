package jobs

// SeverityCounts は 1 スキャンの重大度別 finding 件数。scans.summary_json の中身。
// worker（engine 集計）と api（スコア計算）の双方がこの型を経由することで、
// summary_json の形が別々に定義されてズレる（ドリフト）のを構造的に防ぐ。
type SeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

// ScanSummary は scans.summary_json に保存するエンベロープ。件数を "findings" キーの
// 下にネストする。スコアは持たない（api 側 report が SeverityCounts から算出する）。
type ScanSummary struct {
	Findings SeverityCounts `json:"findings"`
}
