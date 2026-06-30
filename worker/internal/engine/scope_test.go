package engine

import "testing"

func TestNewScope(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		wantErr  bool
		wantHost string
	}{
		{"https ok", "https://example.com", false, "example.com"},
		{"http ok", "http://example.com:8080/path", false, "example.com"},
		{"host lowercased", "https://EXAMPLE.com", false, "example.com"},
		{"ipv6 host", "http://[::1]:3000", false, "::1"},
		{"unsupported scheme", "ftp://example.com", true, ""},
		{"missing host", "https:///path", true, ""},
		{"unparseable", "http://%zz", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScope(tt.baseURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewScope(%q) expected error, got nil", tt.baseURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewScope(%q) unexpected error: %v", tt.baseURL, err)
			}
			if s.Host() != tt.wantHost {
				t.Errorf("Host() = %q, want %q", s.Host(), tt.wantHost)
			}
			if s.BaseURL() != tt.baseURL {
				t.Errorf("BaseURL() = %q, want %q", s.BaseURL(), tt.baseURL)
			}
		})
	}
}

func TestScopeAllows(t *testing.T) {
	s, err := NewScope("https://example.com")
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"same host allowed", "https://example.com/products?id=1", true},
		{"same host other path", "https://example.com/api/users", true},
		{"different host rejected", "https://evil.com/x", false},
		{"subdomain rejected", "https://api.example.com/x", false},
		{"dangerous logout rejected", "https://example.com/account/logout", false},
		{"dangerous admin rejected", "https://example.com/admin/users", false},
		{"unparseable rejected", "http://%zz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.Allows(tt.url); got != tt.want {
				t.Errorf("Allows(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestScopeRequiresOwnershipVerification(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    bool
	}{
		{"public host requires", "https://example.com", true},
		{"localhost skips", "http://localhost:3000", false},
		{"127.0.0.1 skips", "http://127.0.0.1:8080", false},
		{"ipv6 loopback skips", "http://[::1]:3000", false},
		{"dot-local skips", "http://myapp.local", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScope(tt.baseURL)
			if err != nil {
				t.Fatalf("NewScope(%q): %v", tt.baseURL, err)
			}
			if got := s.RequiresOwnershipVerification(); got != tt.want {
				t.Errorf("RequiresOwnershipVerification() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDangerousPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"plain page", "/products", false},
		{"root", "/", false},
		{"logout", "/account/logout", true},
		{"signout", "/signout", true},
		{"delete", "/api/delete", true},
		{"remove", "/cart/remove", true},
		{"destroy", "/session/destroy", true},
		{"admin", "/admin", true},
		{"admin nested", "/admin/settings", true},
		{"case insensitive", "/Account/LogOut", true},
		{"substring not matched", "/logouter", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDangerousPath(tt.path); got != tt.want {
				t.Errorf("IsDangerousPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
