package engine

import (
	"testing"

	"github.com/ymd38/goodast/jobs"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     jobs.SeverityCounts
	}{
		{
			name:     "empty",
			findings: nil,
			want:     jobs.SeverityCounts{},
		},
		{
			name: "one of each severity",
			findings: []Finding{
				{Severity: SeverityCritical},
				{Severity: SeverityHigh},
				{Severity: SeverityMedium},
				{Severity: SeverityLow},
				{Severity: SeverityInfo},
			},
			want: jobs.SeverityCounts{Critical: 1, High: 1, Medium: 1, Low: 1, Info: 1, Total: 5},
		},
		{
			name: "duplicates aggregate",
			findings: []Finding{
				{Severity: SeverityHigh},
				{Severity: SeverityHigh},
				{Severity: SeverityMedium},
			},
			want: jobs.SeverityCounts{High: 2, Medium: 1, Total: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Summarize(tt.findings); got != tt.want {
				t.Errorf("Summarize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
