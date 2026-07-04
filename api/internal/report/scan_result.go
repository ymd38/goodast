package report

import (
	"errors"
	"time"
)

// ErrScanNotFound は指定 scan が存在しないことを表す（handler が 404 に翻訳する）。
// 診断はバックグラウンド実行のため、scan が存在する限り status（queued/running/…）で
// 進捗を伝え、404 は scan 行そのものが無い場合に限る。
var ErrScanNotFound = errors.New("scan not found")

// ScanSummary はスキャン結果のサマリ（重大度別カウント＋算出スコア）。
// summary_json を持たない（未完了）スキャンでは ScanState.Summary は nil になる。
type ScanSummary struct {
	Counts SeverityCounts `json:"counts"`
	Score  int            `json:"score"`
	Band   Band           `json:"band"`
	Label  string         `json:"label"`
}

// ScanState は 1 スキャンの状態（レポート上段・進捗ポーリング兼用）。
// Summary は done で summary_json が入って初めて非 nil になる。
type ScanState struct {
	ID            string       `json:"id"`
	SiteID        string       `json:"site_id"`
	Status        string       `json:"status"`
	EngineVersion string       `json:"engine_version"`
	CreatedAt     time.Time    `json:"created_at"`
	StartedAt     *time.Time   `json:"started_at"`
	FinishedAt    *time.Time   `json:"finished_at"`
	Summary       *ScanSummary `json:"summary"`
}

// Finding はレポート明細 1 件の表示用データ（不変条件を持たない単純データ）。
type Finding struct {
	ID          string `json:"id"`
	TemplateID  string `json:"template_id"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	URL         string `json:"url"`
	CWE         string `json:"cwe"`
	Remediation string `json:"remediation"`
	Status      string `json:"status"`
}

// buildScanSummary は重大度別カウントから ScanSummary（スコア・バンド・ラベル込み）を組み立てる
// 純粋関数。スコア算出は Compute に集約する。
func buildScanSummary(counts SeverityCounts) ScanSummary {
	s := Compute(counts)
	return ScanSummary{
		Counts: counts,
		Score:  s.Value(),
		Band:   s.Band(),
		Label:  s.Label(),
	}
}
