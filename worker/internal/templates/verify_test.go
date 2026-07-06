package templates

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeMarker(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, markerFile), []byte(content), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
}

func TestVerify(t *testing.T) {
	const want = "v10.4.5"
	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns dir to verify
		wantErr bool
	}{
		{
			name:    "dir missing",
			setup:   func(t *testing.T) string { return filepath.Join(t.TempDir(), "does-not-exist") },
			wantErr: true,
		},
		{
			name:    "marker missing",
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: true,
		},
		{
			name: "version mismatch",
			setup: func(t *testing.T) string {
				d := t.TempDir()
				writeMarker(t, d, "v10.0.0")
				return d
			},
			wantErr: true,
		},
		{
			name: "version match",
			setup: func(t *testing.T) string {
				d := t.TempDir()
				writeMarker(t, d, want)
				return d
			},
			wantErr: false,
		},
		{
			name: "version match with surrounding whitespace",
			setup: func(t *testing.T) string {
				d := t.TempDir()
				writeMarker(t, d, "  "+want+"\n")
				return d
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			err := Verify(dir, want)
			if tt.wantErr {
				if !errors.Is(err, ErrTemplatesMissing) {
					t.Fatalf("Verify() err = %v, want ErrTemplatesMissing", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() err = %v, want nil", err)
			}
		})
	}
}
