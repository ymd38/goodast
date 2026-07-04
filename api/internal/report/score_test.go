package report

import "testing"

func TestCompute(t *testing.T) {
	tests := []struct {
		name   string
		counts SeverityCounts
		want   int
	}{
		{"no findings は満点", SeverityCounts{}, 100},
		{"Low 1件で -1", SeverityCounts{Low: 1}, 99},
		{"Medium 1件で -3", SeverityCounts{Medium: 1}, 97},
		{"High 1件で -10", SeverityCounts{High: 1}, 90},
		{"Critical 1件で -40", SeverityCounts{Critical: 1}, 60},
		{"Info は減点しない", SeverityCounts{Info: 5}, 100},
		{"混在の加重和", SeverityCounts{Critical: 1, High: 1, Medium: 1, Low: 1}, 100 - 54},
		{"Critical 2件で -80", SeverityCounts{Critical: 2}, 20},
		{"Critical 3件は 0 にクランプ（負にならない）", SeverityCounts{Critical: 3}, 0},
		{"大量 findings でも 0 止まり", SeverityCounts{Critical: 10, High: 10}, 0},
		{"Total は計算に影響しない", SeverityCounts{Low: 1, Total: 99}, 99},
		{"負数カウント混入は 100 にクランプ（不変条件を守る）", SeverityCounts{Critical: -1}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Compute(tt.counts).Value(); got != tt.want {
				t.Errorf("Compute(%+v).Value() = %d, want %d", tt.counts, got, tt.want)
			}
		})
	}
}

func TestNewScore(t *testing.T) {
	tests := []struct {
		name    string
		v       int
		wantErr bool
	}{
		{"下限 0 は有効", 0, false},
		{"上限 100 は有効", 100, false},
		{"中間値は有効", 55, false},
		{"下限未満はエラー", -1, true},
		{"上限超過はエラー", 101, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScore(tt.v)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewScore(%d) expected error, got nil", tt.v)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewScore(%d) unexpected error: %v", tt.v, err)
			}
			if s.Value() != tt.v {
				t.Errorf("NewScore(%d).Value() = %d, want %d", tt.v, s.Value(), tt.v)
			}
		})
	}
}

func TestScoreBandAndLabel(t *testing.T) {
	tests := []struct {
		name      string
		v         int
		wantBand  Band
		wantLabel string
	}{
		{"満点は good", 100, BandGood, "良好"},
		{"good 下限 80", 80, BandGood, "良好"},
		{"caution 上限 79", 79, BandCaution, "要注意"},
		{"caution 下限 60", 60, BandCaution, "要注意"},
		{"danger 上限 59", 59, BandDanger, "危険"},
		{"danger 下限 40", 40, BandDanger, "危険"},
		{"crisis 上限 39", 39, BandCrisis, "危機"},
		{"最低点 0 は crisis", 0, BandCrisis, "危機"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScore(tt.v)
			if err != nil {
				t.Fatalf("NewScore(%d): %v", tt.v, err)
			}
			if got := s.Band(); got != tt.wantBand {
				t.Errorf("Score(%d).Band() = %q, want %q", tt.v, got, tt.wantBand)
			}
			if got := s.Label(); got != tt.wantLabel {
				t.Errorf("Score(%d).Label() = %q, want %q", tt.v, got, tt.wantLabel)
			}
		})
	}
}

func TestScoreDelta(t *testing.T) {
	tests := []struct {
		name       string
		curr, prev int
		want       int
	}{
		{"改善（正）", 80, 60, 20},
		{"悪化（負）", 40, 70, -30},
		{"変化なし（ゼロ）", 55, 55, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			curr, err := NewScore(tt.curr)
			if err != nil {
				t.Fatalf("NewScore(%d): %v", tt.curr, err)
			}
			prev, err := NewScore(tt.prev)
			if err != nil {
				t.Fatalf("NewScore(%d): %v", tt.prev, err)
			}
			if got := curr.Delta(prev); got != tt.want {
				t.Errorf("Score(%d).Delta(Score(%d)) = %d, want %d", tt.curr, tt.prev, got, tt.want)
			}
		})
	}
}
