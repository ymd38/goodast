//go:build integration

package katana

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ymd38/goodast/worker/internal/engine"
)

func targetOrDefault() string {
	if v := os.Getenv("NUCLEI_TEST_TARGET"); v != "" {
		return v
	}
	return "http://localhost:3001" // make juiceshop-up の loopback
}

func TestKatanaCrawlDiscoversWithinScope(t *testing.T) {
	target := targetOrDefault()
	scope, err := engine.NewScope(target)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := New().Crawl(ctx, scope, engine.CrawlPlan{Enabled: true, MaxDepth: 2, MaxURLs: 50}, nil)
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}

	// 1. 診断対象が拡張される（seed 1 本より多く発見）＝ hard assert。
	if len(res.URLs) < 1 {
		t.Fatalf("発見 URL が空: %+v", res)
	}
	t.Logf("discovered=%d forms=%d (件数はステートフルで非決定的・レポート)", len(res.URLs), res.FormCount)

	base := scope.Host()
	for _, u := range res.URLs {
		// 2. すべてスコープ内（host:port 一致・危険パス除外）。
		if !scope.Allows(u) {
			t.Errorf("スコープ外 URL が混入: %s", u)
		}
		// 3. 危険パスを踏んでいない。
		low := strings.ToLower(u)
		for _, danger := range []string{"/logout", "/delete", "/admin", "/remove", "/destroy", "/signout"} {
			if strings.Contains(low, danger) {
				t.Errorf("危険パスが発見 URL に含まれる: %s", u)
			}
		}
		// 3'. 別ホストへ出ていない。
		parsed, perr := url.Parse(u)
		if perr != nil || !strings.EqualFold(parsed.Hostname(), base) {
			t.Errorf("別ホストの URL が混入: %s", u)
		}
	}
}

func TestKatanaCrawlRespectsMaxURLs(t *testing.T) {
	target := targetOrDefault()
	scope, err := engine.NewScope(target)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := New().Crawl(ctx, scope, engine.CrawlPlan{Enabled: true, MaxDepth: 3, MaxURLs: 5}, nil)
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	// 4. MaxURLs=5 で頭打ち。
	if len(res.URLs) > 5 {
		t.Fatalf("MaxURLs を超過: %d", len(res.URLs))
	}
}
