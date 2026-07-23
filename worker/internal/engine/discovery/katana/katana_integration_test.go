//go:build integration

package katana

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ymd38/goodast/worker/internal/engine"
)

// linkedSite は複数ページがリンクした httptest サーバを返す。/ から products/about/危険パス/
// スコープ外ポートへリンクし、products は子ページを持つ（探索の深さ・スコープ・危険パス除外を検証できる）。
func linkedSite(t *testing.T) *httptest.Server {
	t.Helper()
	page := func(links ...string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			b := "<!doctype html><html><body>"
			for _, l := range links {
				b += fmt.Sprintf(`<a href=%q>x</a>`, l)
			}
			_, _ = w.Write([]byte(b + "</body></html>"))
		}
	}
	mux := http.NewServeMux()
	// 127.0.0.1:9 は別 authority（スコープ外・ポート違い）。/account/logout は危険パス。
	// /style.css は静的アセット（in-scope だが診断対象に入れてはならない）。
	mux.HandleFunc("/", page("/products", "/about", "/account/logout", "/style.css", "http://127.0.0.1:9/evil"))
	mux.HandleFunc("/products", page("/products/1"))
	mux.HandleFunc("/products/1", page())
	mux.HandleFunc("/about", page())
	mux.HandleFunc("/account/logout", page()) // クロールされてはならない
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte("body{}"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestKatanaCrawlFollowsLinks は Katana が実際にリンクを追従して診断対象を拡張すること、
// スコープ外・危険パスを踏まないことを決定的に検証する（外部依存なし）。
// これは「Options を DefaultOptions ベースにしないと BodyReadSize=0 でリンク抽出 0 になる」
// 回帰の門番でもある。
func TestKatanaCrawlFollowsLinks(t *testing.T) {
	srv := linkedSite(t)
	scope, err := engine.NewScope(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := New().Crawl(ctx, scope, engine.CrawlPlan{Enabled: true, MaxDepth: 3, MaxURLs: 50}, nil)
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	t.Logf("discovered=%d: %v", len(res.URLs), res.URLs)

	got := map[string]bool{}
	for _, u := range res.URLs {
		got[u] = true
		if !scope.Allows(u) {
			t.Errorf("スコープ外 URL が混入: %s", u)
		}
		if strings.Contains(strings.ToLower(u), "logout") {
			t.Errorf("危険パスがクロールされた: %s", u)
		}
	}

	// 1. リンク追従で診断対象が拡張している（seed 以外を発見）= BodyReadSize 回帰の門番。
	for _, want := range []string{srv.URL + "/products", srv.URL + "/about", srv.URL + "/products/1"} {
		if !got[want] {
			t.Errorf("発見されるべき URL が無い: %s", want)
		}
	}
	// 2. スコープ外ホスト（別ポート）へ出ていない = post-check の host:port 強制。
	if got["http://127.0.0.1:9/evil"] {
		t.Error("スコープ外 authority が混入した")
	}
	// 3. 静的アセット（.css）は in-scope でも診断対象に入れない。
	if got[srv.URL+"/style.css"] {
		t.Error("静的アセット /style.css が診断対象に混入した")
	}
	if len(res.URLs) < 4 {
		t.Fatalf("探索が拡張していない（len=%d・BodyReadSize 回帰の疑い）", len(res.URLs))
	}
}

// TestKatanaCrawlRespectsMaxURLs は MaxURLs のハード上限を決定的に検証する
// （in-scope ページが上限より多い linkedSite で上限に張り付くこと）。
func TestKatanaCrawlRespectsMaxURLs(t *testing.T) {
	srv := linkedSite(t)
	scope, err := engine.NewScope(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := New().Crawl(ctx, scope, engine.CrawlPlan{Enabled: true, MaxDepth: 3, MaxURLs: 2}, nil)
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if len(res.URLs) > 2 {
		t.Fatalf("MaxURLs=2 を超過: %d (%v)", len(res.URLs), res.URLs)
	}
	t.Logf("capped discovered=%d", len(res.URLs))
}
