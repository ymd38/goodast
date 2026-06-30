package engine

import "testing"

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Severity
	}{
		{"critical", "critical", SeverityCritical},
		{"high", "high", SeverityHigh},
		{"medium", "medium", SeverityMedium},
		{"low", "low", SeverityLow},
		{"info", "info", SeverityInfo},
		{"mixed case", "Critical", SeverityCritical},
		{"surrounding spaces", "  high  ", SeverityHigh},
		{"unknown maps to info", "unknown", SeverityInfo},
		{"empty maps to info", "", SeverityInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseSeverity(tt.in); got != tt.want {
				t.Errorf("ParseSeverity(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
