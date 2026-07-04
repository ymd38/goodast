package report

import "testing"

func TestBuildScanSummary(t *testing.T) {
	tests := []struct {
		name      string
		counts    SeverityCounts
		wantScore int
		wantBand  Band
		wantLabel string
	}{
		{"クリーン（0件）は満点", SeverityCounts{}, 100, BandGood, "良好"},
		{"High 1件 → 90", SeverityCounts{High: 1, Total: 1}, 90, BandGood, "良好"},
		{"Critical 1件 → 60", SeverityCounts{Critical: 1, Total: 1}, 60, BandCaution, "要注意"},
		{"複合 → 危険帯", SeverityCounts{Critical: 1, High: 1, Total: 2}, 50, BandDanger, "危険"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildScanSummary(tt.counts)
			if got.Score != tt.wantScore || got.Band != tt.wantBand || got.Label != tt.wantLabel {
				t.Errorf("buildScanSummary(%+v) = {score:%d band:%s label:%s}, want {score:%d band:%s label:%s}",
					tt.counts, got.Score, got.Band, got.Label, tt.wantScore, tt.wantBand, tt.wantLabel)
			}
			if got.Counts != tt.counts {
				t.Errorf("Counts = %+v, want %+v (入力を保持すべき)", got.Counts, tt.counts)
			}
		})
	}
}
