package engine

import (
	"testing"
	"time"

	"github.com/ymd38/goodast/jobs"
)

func TestPlanFor(t *testing.T) {
	tests := []struct {
		name        string
		preset      jobs.Preset
		wantTags    []string
		wantTimeout time.Duration
	}{
		{
			"light",
			jobs.PresetLight,
			[]string{"misconfig", "tech", "exposure"},
			15 * time.Minute,
		},
		{
			"standard",
			jobs.PresetStandard,
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve"},
			30 * time.Minute,
		},
		{
			"deep",
			jobs.PresetDeep,
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve", "xss", "sqli", "lfi", "ssrf", "rce", "takeover"},
			60 * time.Minute,
		},
		{
			"unknown falls back to standard",
			jobs.Preset("bogus"),
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve"},
			30 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := PlanFor(tt.preset)
			if tt.wantTimeout != plan.Timeout {
				t.Fatalf("timeout = %v, want %v", plan.Timeout, tt.wantTimeout)
			}
			if !equalStrings(plan.Scan.Tags, tt.wantTags) {
				t.Fatalf("tags = %v, want %v", plan.Scan.Tags, tt.wantTags)
			}
			if !equalStrings(plan.Scan.ExcludeTags, []string{"dos", "intrusive"}) {
				t.Fatalf("exclude = %v", plan.Scan.ExcludeTags)
			}
			if plan.Scan.RateLimit != 10 || plan.Scan.RatePeriod != time.Second {
				t.Fatalf("rate = %d/%v", plan.Scan.RateLimit, plan.Scan.RatePeriod)
			}
			if plan.Scan.Severities != "" {
				t.Fatalf("severities = %q", plan.Scan.Severities)
			}
		})
	}
}

func TestScanProfileForLocalTarget(t *testing.T) {
	base := PlanFor(jobs.PresetLight).Scan
	local := base.ForLocalTarget()
	if local.RateLimit != localRateLimit {
		t.Fatalf("local rate = %d, want %d", local.RateLimit, localRateLimit)
	}
	// 外部向けの保守的レートは元のまま（複製が変わっても base は不変）。
	if base.RateLimit != 10 {
		t.Fatalf("base rate mutated = %d, want 10", base.RateLimit)
	}
	// レート以外（タグ・除外・期間）は据え置き。
	if !equalStrings(local.Tags, base.Tags) || !equalStrings(local.ExcludeTags, base.ExcludeTags) || local.RatePeriod != base.RatePeriod {
		t.Fatalf("ForLocalTarget changed more than rate: %+v", local)
	}
}

func TestPlanForCrawlBounds(t *testing.T) {
	tests := []struct {
		name    string
		preset  jobs.Preset
		enabled bool
		depth   int
		maxURLs int
	}{
		{"light はクロール無効", jobs.PresetLight, false, 0, 0},
		{"standard は浅いクロール", jobs.PresetStandard, true, 2, 50},
		{"deep は広いクロール", jobs.PresetDeep, true, 3, 200},
		{"未知は standard 既定", jobs.Preset("bogus"), true, 2, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanFor(tt.preset).Crawl
			if got.Enabled != tt.enabled || got.MaxDepth != tt.depth || got.MaxURLs != tt.maxURLs {
				t.Fatalf("Crawl = %+v; want {Enabled:%v MaxDepth:%d MaxURLs:%d}",
					got, tt.enabled, tt.depth, tt.maxURLs)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
