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
			5 * time.Minute,
		},
		{
			"standard",
			jobs.PresetStandard,
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve"},
			15 * time.Minute,
		},
		{
			"deep",
			jobs.PresetDeep,
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve", "xss", "sqli", "lfi", "ssrf", "rce", "takeover"},
			30 * time.Minute,
		},
		{
			"unknown falls back to standard",
			jobs.Preset("bogus"),
			[]string{"misconfig", "tech", "exposure", "exposed-panels", "default-login", "cve"},
			15 * time.Minute,
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
