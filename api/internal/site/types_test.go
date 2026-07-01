package site

import (
	"errors"
	"testing"
)

func TestParseVerifyMethod(t *testing.T) {
	tests := []struct {
		in      string
		want    VerifyMethod
		wantErr bool
	}{
		{"file", VerifyMethodFile, false},
		{"dns-txt", VerifyMethodDNSTXT, false},
		{"", "", true},
		{"bogus", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseVerifyMethod(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseVerifyMethod(%q) err=%v wantErr=%v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseVerifyMethod(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewVerifyToken(t *testing.T) {
	tok, err := NewVerifyToken()
	if err != nil {
		t.Fatalf("NewVerifyToken: %v", err)
	}
	if len(tok.String()) != verifyTokenBytes*2 {
		t.Errorf("token length = %d, want %d", len(tok.String()), verifyTokenBytes*2)
	}
	if _, err := ParseVerifyToken(tok.String()); err != nil {
		t.Errorf("generated token not parseable: %v", err)
	}
}

func TestNewVerifyTokenRandError(t *testing.T) {
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("rng failure") }
	defer func() { randRead = orig }()

	if _, err := NewVerifyToken(); err == nil {
		t.Fatal("expected error when rng fails, got nil")
	}
}

func TestParseVerifyToken(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef" // 32 hex chars = 16 bytes
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"valid", valid, false},
		{"too short", "abcd", true},
		{"too long", valid + "00", true},
		{"non-hex", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseVerifyToken(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseVerifyToken(%q) err=%v wantErr=%v", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestVerifyTokenEqual(t *testing.T) {
	a, _ := ParseVerifyToken("0123456789abcdef0123456789abcdef")
	same, _ := ParseVerifyToken("0123456789abcdef0123456789abcdef")
	other, _ := ParseVerifyToken("ffffffffffffffffffffffffffffffff")

	if !a.Equal(same) {
		t.Error("Equal returned false for identical tokens")
	}
	if a.Equal(other) {
		t.Error("Equal returned true for different tokens")
	}
}
