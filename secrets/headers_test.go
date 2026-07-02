package secrets

import (
	"errors"
	"strings"
	"testing"
)

func TestHeadersValidate(t *testing.T) {
	tests := []struct {
		name    string
		headers Headers
		wantErr error
	}{
		{"valid", Headers{{Name: "Cookie", Value: "a=b"}, {Name: "Authorization", Value: "Bearer x"}}, nil},
		{"empty set", Headers{}, ErrNoHeaders},
		{"empty name", Headers{{Name: "", Value: "v"}}, ErrInvalidHeader},
		{"space in name", Headers{{Name: "Bad Name", Value: "v"}}, ErrInvalidHeader},
		{"colon in name", Headers{{Name: "Co:okie", Value: "v"}}, ErrInvalidHeader},
		{"newline in name", Headers{{Name: "Cookie\n", Value: "v"}}, ErrInvalidHeader},
		{"CRLF in value", Headers{{Name: "Cookie", Value: "a\r\nInjected: 1"}}, ErrInvalidHeader},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.headers.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestToNucleiFormat(t *testing.T) {
	h := Headers{{Name: "Cookie", Value: "a=b"}, {Name: "Authorization", Value: "Bearer x"}}
	got := h.ToNucleiFormat()
	want := []string{"Cookie: a=b", "Authorization: Bearer x"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHeadersStringMasksValues(t *testing.T) {
	h := Headers{{Name: "Cookie", Value: "secret-token"}, {Name: "Authorization", Value: "Bearer sensitive"}}
	got := h.String()
	if strings.Contains(got, "secret-token") || strings.Contains(got, "sensitive") {
		t.Errorf("String() leaked a value: %q", got)
	}
	if !strings.Contains(got, "Cookie") || !strings.Contains(got, "Authorization") {
		t.Errorf("String() should list names: %q", got)
	}
}
