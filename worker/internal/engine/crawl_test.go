package engine

import (
	"regexp"
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
		{"URL0 は base(最小)", 0, ceiling, 2 * time.Minute},  // base=2m が下限を兼ねる
		{"少数 URL は base+加算", 6, ceiling, 3 * time.Minute}, // 2m + 6*10s = 3m
		{"多数 URL は ceiling で頭打ち", 1000, ceiling, 30 * time.Minute},
		{"ceiling が base+加算 未満でも ceiling を超えない", 5, 1 * time.Minute, 1 * time.Minute},
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

func TestCrawlCollector_Offer(t *testing.T) {
	scope, err := NewScope("http://localhost:3001")
	if err != nil {
		t.Fatal(err)
	}
	otherScope, err := NewScope("http://example.com")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("非 GET かつ in-scope は forms を加算し urls には入れない", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		capped := c.Offer("POST", "http://localhost:3001/login")
		if capped {
			t.Fatal("capped が true")
		}
		res := c.Result()
		if res.FormCount != 1 {
			t.Fatalf("FormCount = %d; want 1", res.FormCount)
		}
		if len(res.URLs) != 0 {
			t.Fatalf("URLs = %v; want empty", res.URLs)
		}
	})

	t.Run("非 GET かつ out-of-scope（別ホスト）は forms を加算しない", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		capped := c.Offer("POST", "http://evil.example.com/login")
		if capped {
			t.Fatal("capped が true")
		}
		res := c.Result()
		if res.FormCount != 0 {
			t.Fatalf("FormCount = %d; want 0", res.FormCount)
		}
	})

	t.Run("非 GET かつ危険パス（out-of-scope）は forms を加算しない", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		capped := c.Offer("POST", "http://localhost:3001/logout")
		if capped {
			t.Fatal("capped が true")
		}
		res := c.Result()
		if res.FormCount != 0 {
			t.Fatalf("FormCount = %d; want 0", res.FormCount)
		}
	})

	t.Run("GET out-of-scope は追加されない", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		capped := c.Offer("GET", "http://evil.example.com/")
		if capped {
			t.Fatal("capped が true")
		}
		res := c.Result()
		if len(res.URLs) != 0 {
			t.Fatalf("URLs = %v; want empty", res.URLs)
		}
	})

	t.Run("GET 危険パス（out-of-scope）は追加されない", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		capped := c.Offer("GET", "http://localhost:3001/admin")
		if capped {
			t.Fatal("capped が true")
		}
		res := c.Result()
		if len(res.URLs) != 0 {
			t.Fatalf("URLs = %v; want empty", res.URLs)
		}
	})

	t.Run("GET in-scope 新規は urls に追加される", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		c.Offer("GET", "http://localhost:3001/a")
		res := c.Result()
		if len(res.URLs) != 1 || res.URLs[0] != "http://localhost:3001/a" {
			t.Fatalf("URLs = %v", res.URLs)
		}
	})

	t.Run("GET 重複は 2 回目が追加されない", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		c.Offer("GET", "http://localhost:3001/a")
		c.Offer("GET", "http://localhost:3001/a")
		res := c.Result()
		if len(res.URLs) != 1 {
			t.Fatalf("URLs = %v; want 1 element (dedup)", res.URLs)
		}
	})

	t.Run("HEAD は GET 相当に扱われ in-scope なら追加される", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		capped := c.Offer("HEAD", "http://localhost:3001/a")
		if capped {
			t.Fatal("capped が true")
		}
		res := c.Result()
		if len(res.URLs) != 1 || res.URLs[0] != "http://localhost:3001/a" {
			t.Fatalf("URLs = %v", res.URLs)
		}
	})

	t.Run("method 小文字も正規化される", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		c.Offer("get", "http://localhost:3001/a")
		res := c.Result()
		if len(res.URLs) != 1 {
			t.Fatalf("URLs = %v; want 1 element", res.URLs)
		}
	})

	t.Run("maxURLs=2 で 3 件目は追加されず capped=true・URLs は 2 件のまま", func(t *testing.T) {
		c := NewCrawlCollector(scope, 2)
		if capped := c.Offer("GET", "http://localhost:3001/a"); capped {
			t.Fatal("1 件目で capped になった")
		}
		if capped := c.Offer("GET", "http://localhost:3001/b"); !capped {
			t.Fatal("ちょうど上限に達する 2 件目で capped=true を期待")
		}
		if capped := c.Offer("GET", "http://localhost:3001/c"); !capped {
			t.Fatal("上限到達後の 3 件目で capped=true を期待")
		}
		res := c.Result()
		if len(res.URLs) != 2 {
			t.Fatalf("URLs = %v; want 2 elements (ハード上限)", res.URLs)
		}
	})

	t.Run("maxURLs=0（無制限）は capped が常に false", func(t *testing.T) {
		c := NewCrawlCollector(scope, 0)
		for i := 0; i < 5; i++ {
			if capped := c.Offer("GET", "http://localhost:3001/p"+string(rune('a'+i))); capped {
				t.Fatalf("i=%d で capped=true", i)
			}
		}
		res := c.Result()
		if len(res.URLs) != 5 {
			t.Fatalf("URLs = %v; want 5 elements", res.URLs)
		}
	})

	t.Run("otherScope に対する Offer は Allows で弾かれる（別スコープの健全性確認）", func(t *testing.T) {
		c := NewCrawlCollector(otherScope, 0)
		capped := c.Offer("GET", "http://localhost:3001/a")
		if capped {
			t.Fatal("capped が true")
		}
		res := c.Result()
		if len(res.URLs) != 0 {
			t.Fatalf("URLs = %v; want empty", res.URLs)
		}
	})
}

func TestDangerousPathRegexes(t *testing.T) {
	pats := DangerousPathRegexes()
	if len(pats) == 0 {
		t.Fatal("regex が空")
	}
	matchAny := func(path string) bool {
		for _, p := range pats {
			if regexp.MustCompile(p).MatchString(path) {
				return true
			}
		}
		return false
	}
	blocked := []string{"/logout", "/user/delete", "/admin/panel", "/account/remove", "/DESTROY"}
	for _, p := range blocked {
		if !matchAny(p) {
			t.Errorf("危険パス %q が regex に一致しない", p)
		}
	}
	allowed := []string{"/", "/products", "/search?q=1", "/administrator-guide"}
	for _, p := range allowed {
		if matchAny(p) {
			t.Errorf("非危険パス %q が誤って一致した", p)
		}
	}
}
