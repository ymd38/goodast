package site

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// httpDoer / dnsResolver は所有確認の外部 I/O を抽象化し、テストで fake 差し替えを可能にする。
// 本番実装はそれぞれ *http.Client / *net.Resolver（LookupTXT を満たす）を用いる。
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type dnsResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// Verifier はドメイン所有確認（ファイル設置 / DNS TXT）を実行する（ADR-0004）。
type Verifier struct {
	http httpDoer
	dns  dnsResolver
}

// NewVerifier は依存を注入して Verifier を生成する（テスト用）。
func NewVerifier(h httpDoer, d dnsResolver) *Verifier {
	return &Verifier{http: h, dns: d}
}

// DefaultVerifier は本番用の Verifier（保守的なタイムアウト付き HTTP クライアント + システムリゾルバ）。
func DefaultVerifier() *Verifier {
	return &Verifier{
		http: &http.Client{Timeout: 10 * time.Second},
		dns:  net.DefaultResolver,
	}
}

// Verify は指定方式で所有確認を行う。成功で nil、不一致・到達不能で error を返す。
func (v *Verifier) Verify(ctx context.Context, method VerifyMethod, baseURL string, token VerifyToken) error {
	switch method {
	case VerifyMethodFile:
		return v.verifyFile(ctx, baseURL, token)
	case VerifyMethodDNSTXT:
		return v.verifyDNS(ctx, baseURL, token)
	default:
		return fmt.Errorf("unsupported verify method %q", method)
	}
}

// verifyFile は /.well-known/goodast-verify/<token> を取得し、内容がトークンと一致するか確認する。
func (v *Verifier) verifyFile(ctx context.Context, baseURL string, token VerifyToken) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	// well-known はドメインルート基準にする（base_url のサブパスは無視する）。
	u.Path = path.Join("/.well-known/goodast-verify", token.String())
	u.RawQuery = ""

	// パース済み *url.URL から直接組み立てる（NewRequestWithContext の到達不能な
	// エラー分岐を避ける）。
	req := (&http.Request{Method: http.MethodGet, URL: u, Header: make(http.Header)}).WithContext(ctx)
	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch verification file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("verification file returned status %d", resp.StatusCode)
	}
	// トークン長 + 余白のみ読む（巨大レスポンスによるリソース枯渇を防ぐ）。
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(verifyTokenBytes*2)+64))
	if err != nil {
		return fmt.Errorf("read verification file: %w", err)
	}
	got, err := ParseVerifyToken(strings.TrimSpace(string(body)))
	if err != nil {
		return fmt.Errorf("verification file content invalid: %w", err)
	}
	if !got.Equal(token) {
		return fmt.Errorf("verification token mismatch")
	}
	return nil
}

// verifyDNS は base_url のホストの TXT レコードに goodast-verify=<token> が存在するか確認する。
func (v *Verifier) verifyDNS(ctx context.Context, baseURL string, token VerifyToken) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("base url %q has no host", baseURL)
	}
	records, err := v.dns.LookupTXT(ctx, host)
	if err != nil {
		return fmt.Errorf("lookup TXT for %s: %w", host, err)
	}
	want := "goodast-verify=" + token.String()
	for _, r := range records {
		if strings.TrimSpace(r) == want {
			return nil
		}
	}
	return fmt.Errorf("no matching goodast-verify TXT record for %s", host)
}
