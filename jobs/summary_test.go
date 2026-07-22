package jobs

import (
	"encoding/json"
	"strings"
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

func TestScanSummaryDiscoveryOmitempty(t *testing.T) {
	// クロール無効時は discovery キーを出さない（後方互換）。
	raw, _ := json.Marshal(ScanSummary{Findings: SeverityCounts{Total: 1}})
	if strings.Contains(string(raw), "discovery") {
		t.Fatalf("discovery が omitempty で出ていない: %s", raw)
	}
	// クロール有効時は url_count / form_count を含む。
	in := ScanSummary{
		Findings:  SeverityCounts{Total: 2},
		Discovery: &DiscoveryInfo{URLCount: 12, FormCount: 3},
	}
	raw2, _ := json.Marshal(in)
	var out ScanSummary
	if err := json.Unmarshal(raw2, &out); err != nil {
		t.Fatal(err)
	}
	if out.Discovery == nil || out.Discovery.URLCount != 12 || out.Discovery.FormCount != 3 {
		t.Fatalf("round-trip mismatch: %+v", out.Discovery)
	}
}
