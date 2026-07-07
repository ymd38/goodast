package engine

import (
	"time"

	"github.com/ymd38/goodast/jobs"
)

// ScanProfile は 1 スキャンの実行パラメータ（テンプレート選択とレート）。
// preset から導出され、ScanRequest 経由でエンジンに渡る。per-scan で可変。
type ScanProfile struct {
	Tags        []string
	ExcludeTags []string
	Severities  string
	RateLimit   int
	RatePeriod  time.Duration
}

// Plan は preset から導出したスキャン実行計画。Scan はエンジンへ、Timeout は
// river ジョブの実行上限（scanjob.Worker.Timeout）へ渡る。
type Plan struct {
	Scan    ScanProfile
	Timeout time.Duration
}

// 破壊的テンプレートは全 preset で除外（ADR / Critical Constraints）。
var excludeDestructive = []string{"dos", "intrusive"}

// PlanFor は preset を実行計画に写像する。未知・空 preset は安全側として standard を返す
// （api で検証済みのため通常は起きないが、防御的に既定へ倒す）。
func PlanFor(p jobs.Preset) Plan {
	switch p {
	case jobs.PresetLight:
		return plan([]string{"misconfig", "tech", "exposure"}, 15*time.Minute)
	case jobs.PresetDeep:
		return plan([]string{
			"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve",
			"xss", "sqli", "lfi", "ssrf", "rce", "takeover",
		}, 60*time.Minute)
	default: // standard を既定に
		return plan([]string{
			"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve",
		}, 30*time.Minute)
	}
}

// localRateLimit はローカル/自己所有対象に用いる緩めのレート（req/RatePeriod）。
// 外部対象には CLAUDE.md の Critical Constraint「保守的なデフォルトレート」を維持し、
// localhost 等（ADR-0004 のスキップ対象）に限って ForLocalTarget で引き上げる。
const localRateLimit = 100

// ForLocalTarget はローカル対象向けにレートを引き上げた複製を返す（純粋関数）。
// タグ・タイムアウト等は変えない。呼び出し側（worker）が scope のローカル判定で適用する。
func (p ScanProfile) ForLocalTarget() ScanProfile {
	p.RateLimit = localRateLimit
	return p
}

// plan は共通のレート・除外タグを載せた Plan を組み立てる（DRY）。
func plan(tags []string, timeout time.Duration) Plan {
	return Plan{
		Scan: ScanProfile{
			Tags:        tags,
			ExcludeTags: excludeDestructive,
			Severities:  "",
			RateLimit:   10,
			RatePeriod:  time.Second,
		},
		Timeout: timeout,
	}
}
