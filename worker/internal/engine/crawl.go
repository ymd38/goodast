package engine

import "time"

// CrawlPlan はクロール段の有界化パラメータ。preset から導出される。
// Enabled=false のとき crawler は呼ばれず、診断は単一 URL のまま（現状維持）。
type CrawlPlan struct {
	Enabled  bool
	MaxDepth int
	MaxURLs  int
}

// 動的タイムアウトの係数（spec §6.1・実測でチューニング可能）。
const (
	scanTimeoutBase   = 2 * time.Minute  // 単一 URL でも確保する下駄
	scanTimeoutPerURL = 10 * time.Second // 発見 URL 1 本あたりの追加枠
	scanTimeoutFloor  = 2 * time.Minute
)

// ScanTimeout は発見 URL 数から scan 段の実行枠を算出する。
// base + numURLs×perURL を [floor, ceiling] にクランプする。ceiling は preset 別の絶対上限
// （= PlanFor(preset).Timeout）で、river ジョブ Timeout と一致させる（spec §6.1）。
func ScanTimeout(numURLs int, ceiling time.Duration) time.Duration {
	d := scanTimeoutBase + time.Duration(numURLs)*scanTimeoutPerURL
	if d < scanTimeoutFloor {
		d = scanTimeoutFloor
	}
	if d > ceiling {
		d = ceiling
	}
	return d
}
