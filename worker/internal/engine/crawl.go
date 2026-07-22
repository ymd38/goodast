package engine

import "time"

// CrawlPlan はクロール段の有界化パラメータ。preset から導出される。
// Enabled=false のとき crawler は呼ばれず、診断は単一 URL のまま（現状維持）。
type CrawlPlan struct {
	Enabled  bool
	MaxDepth int
	MaxURLs  int
}

// TargetsOrBase は診断対象 URL を返す。クロールで発見した Targets があればそれを、
// 無ければ Scope.BaseURL() 単一を返す（未クロール時の後方互換）。
func TargetsOrBase(req ScanRequest) []string {
	if len(req.Targets) > 0 {
		return req.Targets
	}
	return []string{req.Scope.BaseURL()}
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

// DangerousPathRegexes は危険パスセグメントを Katana の OutOfScope 正規表現として返す。
// scope.go の dangerousPathSegments を唯一の正とし、セグメント境界（/ または末尾・クエリ）で
// 区切る（`/administrator-guide` の admin 部分一致は弾く。IsDangerousPath と整合）。
func DangerousPathRegexes() []string {
	pats := make([]string, 0, len(dangerousPathSegments))
	for _, seg := range dangerousPathSegments {
		pats = append(pats, `(?i)/`+seg+`(/|$|\?)`)
	}
	return pats
}
