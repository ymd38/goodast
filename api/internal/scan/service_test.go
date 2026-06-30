package scan

import "testing"

func TestIsLocalTarget(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"app.local", true},
		{"foo.bar.local", true},
		{"example.com", false},
		{"localhost.example.com", false},
		{"192.168.0.1", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isLocalTarget(tt.host); got != tt.want {
				t.Errorf("isLocalTarget(%q) = %v, want %v", tt.host, got, tt.want)
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
		{"localhost skips", "http://localhost:8080", false, false},
		{"loopback ip skips", "http://127.0.0.1", false, false},
		{"ipv6 loopback skips", "http://[::1]:3000", false, false},
		{"dot-local skips", "https://juice.local", false, false},
		{"public requires", "https://example.com", true, false},
		{"public with port requires", "https://example.com:8443/path", true, false},
		{"invalid url errors", "http://%zz", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requiresOwnershipVerification(tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("requiresOwnershipVerification(%q) err = %v, wantErr %v", tt.baseURL, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("requiresOwnershipVerification(%q) = %v, want %v", tt.baseURL, got, tt.want)
			}
		})
	}
}
