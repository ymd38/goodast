package target

import "testing"

func TestCanonicalOrigin(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
		wantErr bool
	}{
		{"http explicit port", "http://localhost:3000", "localhost:3000", false},
		{"http default port", "http://example.com", "example.com:80", false},
		{"http default port with path", "http://example.com/app", "example.com:80", false},
		{"https default port", "https://example.com", "example.com:443", false},
		{"https explicit port", "https://example.com:8443/x", "example.com:8443", false},
		{"host uppercased folds", "http://Example.COM:3000", "example.com:3000", false},
		{"127.0.0.1 folds to localhost", "http://127.0.0.1:3001", "localhost:3001", false},
		{"ipv6 loopback folds to localhost", "http://[::1]:3000", "localhost:3000", false},
		{"public ipv6 kept", "http://[2001:db8::1]:80", "[2001:db8::1]:80", false},
		{"unsupported scheme errors", "ftp://example.com", "", true},
		{"missing host errors", "https:///path", "", true},
		{"unparseable errors", "http://%zz", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalOrigin(tt.baseURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("CanonicalOrigin(%q) expected error, got %q", tt.baseURL, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("CanonicalOrigin(%q) unexpected error: %v", tt.baseURL, err)
			}
			if got != tt.want {
				t.Errorf("CanonicalOrigin(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		wantOrigin   string
		wantRequires bool
		wantErr      bool
	}{
		{"public requires verification", "https://example.com", "example.com:443", true, false},
		{"localhost skips verification", "http://localhost:3000", "localhost:3000", false, false},
		{"127.0.0.1 folds and skips", "http://127.0.0.1:3001", "localhost:3001", false, false},
		{"dot-local skips", "http://myapp.local:8080", "myapp.local:8080", false, false},
		{"invalid scheme errors (requires=true)", "ftp://example.com", "", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin, requires, err := Classify(tt.baseURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Classify(%q) expected error", tt.baseURL)
				}
				if !requires {
					t.Errorf("Classify(%q) on error requires=false, want true (safe default)", tt.baseURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("Classify(%q) unexpected error: %v", tt.baseURL, err)
			}
			if origin != tt.wantOrigin || requires != tt.wantRequires {
				t.Errorf("Classify(%q) = (%q, %v), want (%q, %v)", tt.baseURL, origin, requires, tt.wantOrigin, tt.wantRequires)
			}
		})
	}
}

func TestNewSelfOrigins(t *testing.T) {
	t.Run("parses, canonicalizes, and skips empty entries", func(t *testing.T) {
		got, err := NewSelfOrigins([]string{"localhost:3000", " 127.0.0.1:8080 ", "", "[::1]:9000"})
		if err != nil {
			t.Fatalf("NewSelfOrigins: %v", err)
		}
		// 127.0.0.1 と ::1 は localhost に畳み込まれる。
		for _, want := range []string{"localhost:3000", "localhost:8080", "localhost:9000"} {
			if !got.Blocks(want) {
				t.Errorf("Blocks(%q) = false, want true", want)
			}
		}
		if got.Blocks("localhost:1234") {
			t.Error("Blocks(localhost:1234) = true, want false")
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			entries []string
		}{
			{"missing port", []string{"localhost"}},
			{"empty host", []string{":3000"}},
			{"empty port", []string{"localhost:"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := NewSelfOrigins(tt.entries); err == nil {
					t.Fatalf("NewSelfOrigins(%v) expected error", tt.entries)
				}
			})
		}
	})
}

func TestIsLocalTarget(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{"localhost", "localhost", true},
		{"localhost uppercase", "LOCALHOST", true},
		{"ipv4 loopback", "127.0.0.1", true},
		{"ipv6 loopback", "::1", true},
		{"dot-local", "myapp.local", true},
		{"dot-local uppercase", "MyApp.LOCAL", true},
		{"public host", "example.com", false},
		{"subdomain of local-ish", "local.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLocalTarget(tt.host); got != tt.want {
				t.Errorf("IsLocalTarget(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestRequiresOwnershipVerification(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    bool
		wantErr bool
	}{
		{"public requires", "https://example.com", true, false},
		{"public with path", "https://example.com/app", true, false},
		{"localhost skips", "http://localhost:3000", false, false},
		{"127.0.0.1 skips", "http://127.0.0.1:8080", false, false},
		{"ipv6 loopback skips", "http://[::1]:3000", false, false},
		{"dot-local skips", "http://myapp.local", false, false},
		{"unsupported scheme errors", "ftp://example.com", true, true},
		{"missing host errors", "https:///path", true, true},
		{"unparseable errors", "http://%zz", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RequiresOwnershipVerification(tt.baseURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("RequiresOwnershipVerification(%q) expected error, got nil", tt.baseURL)
				}
			} else if err != nil {
				t.Fatalf("RequiresOwnershipVerification(%q) unexpected error: %v", tt.baseURL, err)
			}
			if got != tt.want {
				t.Errorf("RequiresOwnershipVerification(%q) = %v, want %v", tt.baseURL, got, tt.want)
			}
		})
	}
}
