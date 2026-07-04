package report

import (
	"testing"
	"time"
)

var (
	t0 = time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	t1 = time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
)

func TestBuildDashboardEmpty(t *testing.T) {
	d := BuildDashboard(nil)
	if d.Latest != nil {
		t.Errorf("Latest = %+v, want nil", d.Latest)
	}
	if d.History == nil {
		t.Error("History = nil, want non-nil empty slice")
	}
	if len(d.History) != 0 {
		t.Errorf("len(History) = %d, want 0", len(d.History))
	}
}

func TestBuildDashboardSingle(t *testing.T) {
	// High 1件 → スコア 90（良好）。前回が無いので Delta は nil。
	d := BuildDashboard([]ScanPoint{
		{ScanID: "s1", Date: t0, Counts: SeverityCounts{High: 1, Total: 1}},
	})

	if len(d.History) != 1 {
		t.Fatalf("len(History) = %d, want 1", len(d.History))
	}
	if got := d.History[0]; got.Score != 90 || got.Band != BandGood || got.ScanID != "s1" {
		t.Errorf("History[0] = %+v, want score=90 band=good scan=s1", got)
	}

	if d.Latest == nil {
		t.Fatal("Latest = nil, want non-nil")
	}
	if d.Latest.Score != 90 || d.Latest.Band != BandGood || d.Latest.Label != "良好" || d.Latest.ScanID != "s1" {
		t.Errorf("Latest = %+v, want score=90 band=good label=良好 scan=s1", d.Latest)
	}
	if d.Latest.Delta != nil {
		t.Errorf("Latest.Delta = %v, want nil (初回スキャン)", *d.Latest.Delta)
	}
}

func TestBuildDashboardMultiple(t *testing.T) {
	// 昇順: 100（good）→ 90（good）→ 60（caution）。最新は 60、前回差分は 60-90 = -30。
	d := BuildDashboard([]ScanPoint{
		{ScanID: "a", Date: t0, Counts: SeverityCounts{}},
		{ScanID: "b", Date: t1, Counts: SeverityCounts{High: 1}},
		{ScanID: "c", Date: t2, Counts: SeverityCounts{Critical: 1}},
	})

	wantScores := []int{100, 90, 60}
	if len(d.History) != len(wantScores) {
		t.Fatalf("len(History) = %d, want %d", len(d.History), len(wantScores))
	}
	for i, want := range wantScores {
		if d.History[i].Score != want {
			t.Errorf("History[%d].Score = %d, want %d", i, d.History[i].Score, want)
		}
	}
	// 昇順が保たれていること（折れ線 左→右）。
	if !d.History[0].Date.Before(d.History[2].Date) {
		t.Error("History is not in ascending date order")
	}

	if d.Latest == nil {
		t.Fatal("Latest = nil, want non-nil")
	}
	if d.Latest.ScanID != "c" || d.Latest.Score != 60 || d.Latest.Band != BandCaution || d.Latest.Label != "要注意" {
		t.Errorf("Latest = %+v, want scan=c score=60 band=caution label=要注意", d.Latest)
	}
	if d.Latest.Delta == nil {
		t.Fatal("Latest.Delta = nil, want -30")
	}
	if *d.Latest.Delta != -30 {
		t.Errorf("Latest.Delta = %d, want -30", *d.Latest.Delta)
	}
}

func TestBuildDashboardDeltaSign(t *testing.T) {
	tests := []struct {
		name          string
		prev, current SeverityCounts
		wantDelta     int
	}{
		{"改善（正）", SeverityCounts{Critical: 1}, SeverityCounts{}, 40},        // 60 → 100
		{"悪化（負）", SeverityCounts{}, SeverityCounts{Critical: 1}, -40},       // 100 → 60
		{"変化なし（ゼロ）", SeverityCounts{High: 1}, SeverityCounts{High: 1}, 0}, // 90 → 90
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := BuildDashboard([]ScanPoint{
				{ScanID: "prev", Date: t0, Counts: tt.prev},
				{ScanID: "curr", Date: t1, Counts: tt.current},
			})
			if d.Latest == nil || d.Latest.Delta == nil {
				t.Fatalf("Latest/Delta = nil, want delta=%d", tt.wantDelta)
			}
			if *d.Latest.Delta != tt.wantDelta {
				t.Errorf("Delta = %d, want %d", *d.Latest.Delta, tt.wantDelta)
			}
		})
	}
}
