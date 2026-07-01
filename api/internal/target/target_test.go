package target

import "testing"

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
