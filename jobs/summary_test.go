package jobs

import (
	"encoding/json"
	"testing"
)

func TestScanSummaryRoundTrip(t *testing.T) {
	in := ScanSummary{Findings: SeverityCounts{
		Critical: 1, High: 2, Medium: 3, Low: 4, Info: 5, Total: 15,
	}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ScanSummary
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, in)
	}
}

func TestScanSummaryWireShapeIsNested(t *testing.T) {
	raw, _ := json.Marshal(ScanSummary{Findings: SeverityCounts{Critical: 1}})
	// worker が書き api が読む形を固定: {"findings":{"critical":1,...}}
	var probe struct {
		Findings struct {
			Critical int `json:"critical"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("unmarshal probe: %v", err)
	}
	if probe.Findings.Critical != 1 {
		t.Fatalf("expected nested findings.critical=1, got %d", probe.Findings.Critical)
	}
}
