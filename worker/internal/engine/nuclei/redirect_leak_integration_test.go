//go:build integration

package nuclei_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	nucleilib "github.com/projectdiscovery/nuclei/v3/lib"
	nucleitypes "github.com/projectdiscovery/nuclei/v3/pkg/types"
)

// redirectProbeTemplate は redirect を追従する最小テンプレート。target への 1 リクエストが
// クロスホスト 302 で leak サーバへ飛ぶ状況を作る。matcher の成否に依らず、leak サーバへ
// リクエストが届いた時点でヘッダ受信を記録するため、追従が起きたかどうかを確実に観測できる。
const redirectProbeTemplate = `id: goodast-redirect-probe
info:
  name: goodast redirect probe
  author: goodast
  severity: info
http:
  - method: GET
    path:
      - "{{BaseURL}}/probe"
    redirects: true
    max-redirects: 2
    matchers-condition: or
    matchers:
      - type: status
        status:
          - 200
          - 302
`

// TestNucleiAuthHeaderNoCrossHostLeak は W3 対策（認証ヘッダのクロスホスト redirect 漏えい防止）を
// 実 SDK で検証する。target は全リクエストを別ホスト（別ポート）の leak サーバへ 302 redirect し、
// leak サーバは注入ヘッダの受信を記録する。
//
//   - DisableRedirects 無し: redirect が追従され leak サーバへヘッダが届く（＝漏えいを再現）。
//   - DisableRedirects あり（本対策）: redirect が止まり leak サーバへヘッダが一切届かない。
//
// カスタム header 名（Authorization/Cookie 以外）を使うのは、Go 標準の cross-host redirect 時の
// sensitive ヘッダ剥がしにマスクされず、対策の効果を決定的に観測するため（対策は header 名に依らず
// redirect 自体を止めるので任意の認証ヘッダに有効）。
func TestNucleiAuthHeaderNoCrossHostLeak(t *testing.T) {
	if os.Getenv("NUCLEI_TEST_TARGET") == "" {
		t.Skip("NUCLEI_TEST_TARGET not set; skipping nuclei redirect leak test")
	}

	const sentinelName = "X-Goodast-Auth"
	sentinel := "goodast-" + randToken(t)

	var (
		mu     sync.Mutex
		leaked bool
	)
	// leak サーバ（別ホスト扱い）: 注入ヘッダを受信したら記録する。
	leak := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(sentinelName) == sentinel {
			mu.Lock()
			leaked = true
			mu.Unlock()
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer leak.Close()

	// target サーバ: 全リクエストをクロスホストで leak へ 302 redirect する。
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, leak.URL+"/leak", http.StatusFound)
	}))
	defer target.Close()

	tmplPath := filepath.Join(t.TempDir(), "redirect.yaml")
	if err := os.WriteFile(tmplPath, []byte(redirectProbeTemplate), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	// run は target を 1 回スキャンし、leak サーバが注入ヘッダを受信したか返す。
	// disableRedirects が true のとき本対策（nuclei.go の認証時挙動）と同じ WithOptions を適用する。
	run := func(disableRedirects bool) bool {
		mu.Lock()
		leaked = false
		mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		opts := []nucleilib.NucleiSDKOptions{
			nucleilib.WithTemplatesOrWorkflows(nucleilib.TemplateSources{Templates: []string{tmplPath}}),
			nucleilib.WithSandboxOptions(false, false),
			nucleilib.DisableUpdateCheck(),
			nucleilib.WithHeaders([]string{sentinelName + ": " + sentinel}),
		}
		if disableRedirects {
			base := nucleitypes.DefaultOptions()
			base.DisableRedirects = true
			opts = append([]nucleilib.NucleiSDKOptions{nucleilib.WithOptions(base)}, opts...)
		}

		ne, err := nucleilib.NewNucleiEngineCtx(ctx, opts...)
		if err != nil {
			t.Fatalf("create nuclei engine (disableRedirects=%v): %v", disableRedirects, err)
		}
		defer ne.Close()

		ne.LoadTargets([]string{target.URL}, false)
		if err := ne.ExecuteCallbackWithCtx(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("execute scan (disableRedirects=%v): %v", disableRedirects, err)
		}

		mu.Lock()
		defer mu.Unlock()
		return leaked
	}

	// 1) 対策なし: クロスホスト redirect でヘッダが漏れることを確認する（シナリオとテストが有効な証拠）。
	if !run(false) {
		t.Fatal("redirect 追従で leak サーバへヘッダが届かなかった: テンプレ未ロード等でシナリオが成立していない可能性")
	}
	// 2) 対策あり: 認証ヘッダがクロスホスト先へ一切届かないこと。
	if run(true) {
		t.Error("W3: DisableRedirects 下でも認証ヘッダがクロスホスト redirect 先へ漏えいした")
	}
}
