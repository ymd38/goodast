package jobs

import (
	"errors"
	"testing"
)

func TestParsePreset(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Preset
		wantErr error
	}{
		{"light", "light", PresetLight, nil},
		{"standard", "standard", PresetStandard, nil},
		{"deep", "deep", PresetDeep, nil},
		{"empty defaults to standard", "", PresetStandard, nil},
		{"unknown", "aggressive", "", ErrInvalidPreset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePreset(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPresetString(t *testing.T) {
	if PresetDeep.String() != "deep" {
		t.Fatalf("String() = %q", PresetDeep.String())
	}
}
