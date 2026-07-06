//go:build integration

package nuclei_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ymd38/goodast/jobs"
	"github.com/ymd38/goodast/worker/internal/engine"
	"github.com/ymd38/goodast/worker/internal/engine/nuclei"
)

// TestNucleiHeaderInjection は ADR-0003 の認証ヘッダ注入が「実 SDK 経由でワイヤーまで到達する」
// ことを決定的に検証する。Juice Shop の内容やテンプレートの検出件数に依存しないよう、
// 検証用のローカル HTTP キャプチャサーバを立て、そこへ goodast エンジンでスキャンをかけ、
// nuclei が送出したリクエストに注入ヘッダが乗っていることを直接確認する。
//
// §10-3（認証後スキャン）の中核は「持ち込みセッションが確かにリクエストへ付与される」ことであり、
// 本テストはそれを非フレークに担保する（TestNucleiAuthenticatedCoverage は Juice Shop 相手の
// end-to-end 補完）。
//
// 前提: 統合環境（nuclei-templates 導入済み）。NUCLEI_TEST_TARGET を gate に用いる
// （対象自体は本テスト内のキャプチャサーバを使うが、テンプレート導入済み環境の proxy として揃える）。
func TestNucleiHeaderInjection(t *testing.T) {
	if os.Getenv("NUCLEI_TEST_TARGET") == "" {
		t.Skip("NUCLEI_TEST_TARGET not set; skipping nuclei header injection test")
	}

	// 注入を一意に識別するための sentinel ヘッダ（実際の認証情報は使わない）。
	const sentinelName = "X-Goodast-Auth"
	sentinelValue := "goodast-" + randToken(t)

	ctx, cancelScan := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelScan()

	var (
		mu       sync.Mutex
		captured bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(sentinelName) == sentinelValue {
			mu.Lock()
			if !captured {
				captured = true
				cancelScan() // 注入を確認できたらスキャンを早期停止して高速化する
			}
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body>goodast injection probe</body></html>")
	}))
	defer srv.Close()

	scope, err := engine.NewScope(srv.URL)
	if err != nil {
		t.Fatalf("NewScope(%q): %v", srv.URL, err)
	}

	// misconfig タグはヘッダ検査系の軽量テンプレートが GET を送るため注入確認が速い。
	profile := engine.PlanFor(jobs.PresetLight).Scan
	profile.Tags = []string{"misconfig"}

	eng := nuclei.New()
	req := engine.ScanRequest{Scope: scope, Headers: []string{sentinelName + ": " + sentinelValue}, Profile: profile}
	// 注入確認後の早期 cancel（Canceled）と時間切れ（DeadlineExceeded）はどちらも想定内。
	if err := eng.Scan(ctx, req, func(engine.Finding) {}); err != nil &&
		!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("goodast Scan: %v", err)
	}

	mu.Lock()
	got := captured
	mu.Unlock()
	if !got {
		t.Fatalf("注入ヘッダ %q がキャプチャサーバへのどのリクエストにも観測されませんでした: "+
			"ADR-0003 のヘッダ注入がワイヤーに届いていない", sentinelName)
	}
}

// TestNucleiAuthenticatedCoverage は §10-3 の end-to-end 検証。Juice Shop へ実ログインして
// 得た JWT を注入した認証後スキャンが、未認証スキャンのカバレッジを縮小させないことを確認する。
//
// 判定の粒度（フレーク耐性）:
//   - ハード fail: 認証スキャンの template-id 集合が未認証集合を包含しないこと
//     （認証でカバレッジが縮む＝注入がスキャンを壊した兆候。B ⊇ A を不変条件とする）。
//   - レポートのみ: 追加された template-id・finding 件数増。nuclei の公式テンプレートは未認証の
//     フィンガープリント/ヘッダ検査が主体で、選択タグ次第で「認証で増える」件数は非決定的なため
//     （企画書 §10 の「認証後で finding 増」は、認証依存テンプレートが選択集合に入るときに現れる）。
func TestNucleiAuthenticatedCoverage(t *testing.T) {
	target := os.Getenv("NUCLEI_TEST_TARGET")
	if target == "" {
		t.Skip("NUCLEI_TEST_TARGET not set; skipping nuclei authenticated coverage test")
	}
	tags := os.Getenv("NUCLEI_TEST_TAGS")
	if tags == "" {
		tags = "misconfig,tech"
	}
	tagList := splitTags(t, tags)

	scope, err := engine.NewScope(target)
	if err != nil {
		t.Fatalf("NewScope(%q): %v", target, err)
	}
	profile := engine.PlanFor(jobs.PresetLight).Scan
	profile.Tags = tagList

	// --- 未認証スキャン ---
	unauth := runGoodastScan(t, scope, profile, nil)

	// --- 認証後スキャン（Juice Shop に実ログインして JWT を注入）---
	bearer := juiceShopBearer(t, target)
	auth := runGoodastScan(t, scope, profile, []string{"Authorization: Bearer " + bearer})

	unauthSet := templateSet(unauth)
	authSet := templateSet(auth)

	// 未認証集合が空だと B ⊇ A は vacuously true になり、テンプレ未導入・タグ誤設定・対象未到達
	// （エラーにならないケース）でも素通りする。比較の土台＝未認証ベースラインが無い状態を明示 fail する
	// （parity テストの in-scope 0 件ガードと同趣旨）。
	if len(unauthSet) == 0 {
		t.Fatalf("未認証スキャンが template-id を1つも検出しませんでした: "+
			"カバレッジ比較の土台が無く B ⊇ A を検証できない — templates/target/tags を確認 "+
			"(target=%s tags=%v)", target, tagList)
	}

	// B ⊇ A: 認証で未認証の検出クラスを失っていないこと。
	var missing []string
	for id := range unauthSet {
		if !authSet[id] {
			missing = append(missing, id)
		}
	}
	missing = dedupSorted(missing)
	added := extraTemplates(authSet, unauthSet) // auth のみで出た template-id（カバレッジ拡大分）

	t.Logf("unauthenticated: %d findings / %d template-ids", len(unauth), len(unauthSet))
	t.Logf("authenticated:   %d findings / %d template-ids", len(auth), len(authSet))
	t.Logf("coverage delta: findings %+d, template-ids +%d %s",
		len(auth)-len(unauth), len(added), strings.Join(added, ", "))

	if len(missing) > 0 {
		t.Errorf("カバレッジ縮小: 認証スキャンが未認証で検出した %d 個の template-id を欠いています:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(added) == 0 {
		t.Logf("注記: 認証で新規 template-id は増えていません（選択タグ %v は未認証テンプレ主体のため想定内。"+
			"認証依存テンプレートを含めるにはタグを調整する。§10-3 の件数増は report 扱い）", tagList)
	}
}

// juiceShopBearer は Juice Shop に使い捨てユーザーを登録してログインし、JWT を返す。
// 取得した JWT は認証情報のため一切ログしない（Critical Constraints / ADR-0003）。
func juiceShopBearer(t *testing.T, target string) string {
	t.Helper()
	base := strings.TrimRight(target, "/")
	email := fmt.Sprintf("goodast-%s@juice-sh.op", randToken(t))
	password := "Goodast-" + randToken(t)

	// 登録。securityQuestion/Answer はバージョンによって必須なため付与する（余剰フィールドは無視される）。
	regBody, err := json.Marshal(map[string]any{
		"email":            email,
		"password":         password,
		"passwordRepeat":   password,
		"securityQuestion": map[string]any{"id": 1},
		"securityAnswer":   "goodast",
	})
	if err != nil {
		t.Fatalf("marshal register body: %v", err)
	}
	httpPostJSON(t, base+"/api/Users", regBody)

	// ログインして JWT を取得。
	loginBody, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		t.Fatalf("marshal login body: %v", err)
	}
	respBody := httpPostJSON(t, base+"/rest/user/login", loginBody)

	var lr struct {
		Authentication struct {
			Token string `json:"token"`
		} `json:"authentication"`
	}
	if err := json.Unmarshal(respBody, &lr); err != nil {
		t.Fatalf("parse login response: %v", err)
	}
	if lr.Authentication.Token == "" {
		// トークンは含まれない失敗レスポンスなので本文を出しても認証情報は漏れない。
		t.Fatalf("Juice Shop login returned no token (body=%s)", truncateBody(respBody))
	}
	return lr.Authentication.Token
}

// httpPostJSON は JSON body を POST し、2xx のレスポンス本文を返す。非2xx は fail。
func httpPostJSON(t *testing.T, url string, body []byte) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s response: %v", url, err)
	}
	if resp.StatusCode >= 300 {
		t.Fatalf("POST %s: status %d (body=%s)", url, resp.StatusCode, truncateBody(data))
	}
	return data
}

// randToken は衝突しない使い捨て識別子（16 hex 文字）を返す。
func randToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}

// truncateBody はエラーログ用にレスポンス本文を短く切り詰める（巨大 HTML の垂れ流し防止）。
func truncateBody(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
