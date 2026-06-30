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
	tests := []struct {
		name    string
		baseURL string
		url     string
		want    bool
	}{
		// 既定ポート（https=443）の補完: ポート省略表記の差異を吸収する。
		{"same host allowed", "https://example.com", "https://example.com/products?id=1", true},
		{"same host other path", "https://example.com", "https://example.com/api/users", true},
		{"explicit default port matches implicit", "https://example.com", "https://example.com:443/x", true},
		{"implicit default port matches explicit", "https://example.com:443", "https://example.com/x", true},
		// 既定ポート（http=80）の補完。
		{"http default port", "http://example.com", "http://example.com:80/x", true},
		// ホスト不一致・サブドメイン。
		{"different host rejected", "https://example.com", "https://evil.com/x", false},
		{"subdomain rejected", "https://example.com", "https://api.example.com/x", false},
		// ポート境界: 同一ホストでも別ポートは拒否（#8）。
		{"different port rejected", "http://localhost:3001", "http://localhost:3000/x", false},
		{"same explicit port allowed", "http://localhost:3001", "http://localhost:3001/x", true},
		{"non-default port mismatch", "https://example.com", "https://example.com:8443/x", false},
		// 危険パス。
		{"dangerous logout rejected", "https://example.com", "https://example.com/account/logout", false},
		{"dangerous admin rejected", "https://example.com", "https://example.com/admin/users", false},
		// 不正な検出 URL。
		{"unparseable rejected", "https://example.com", "http://%zz", false},
		{"non-http scheme rejected", "https://example.com", "ftp://example.com/x", false},
		{"relative url rejected", "https://example.com", "/relative/path", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScope(tt.baseURL)
			if err != nil {
				t.Fatalf("NewScope(%q): %v", tt.baseURL, err)
			}
			if got := s.Allows(tt.url); got != tt.want {
				t.Errorf("Allows(%q) [base %q] = %v, want %v", tt.url, tt.baseURL, got, tt.want)
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
