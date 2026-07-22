package engine

import (
	"testing"
	"time"
)

func TestScanTimeout(t *testing.T) {
	const ceiling = 30 * time.Minute
	tests := []struct {
		name    string
		numURLs int
		ceiling time.Duration
		want    time.Duration
	}{
		{"URL0 は floor", 0, ceiling, 2 * time.Minute},     // base=2m, floor=2m
		{"少数 URL は base+加算", 6, ceiling, 3 * time.Minute}, // 2m + 6*10s = 3m
		{"多数 URL は ceiling で頭打ち", 1000, ceiling, 30 * time.Minute},
		{"ceiling が floor 未満でも ceiling を超えない", 5, 1 * time.Minute, 1 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScanTimeout(tt.numURLs, tt.ceiling); got != tt.want {
				t.Fatalf("ScanTimeout(%d, %v) = %v; want %v", tt.numURLs, tt.ceiling, got, tt.want)
			}
		})
	}
}

func TestTargetsOrBase(t *testing.T) {
	scope, err := NewScope("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("Targets 空なら base URL", func(t *testing.T) {
		got := TargetsOrBase(ScanRequest{Scope: scope})
		if len(got) != 1 || got[0] != "https://example.com" {
			t.Fatalf("got %v; want [https://example.com]", got)
		}
	})
	t.Run("Targets 非空ならそのまま", func(t *testing.T) {
		in := []string{"https://example.com/a", "https://example.com/b"}
		got := TargetsOrBase(ScanRequest{Scope: scope, Targets: in})
		if len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
			t.Fatalf("got %v; want %v", got, in)
		}
	})
}
