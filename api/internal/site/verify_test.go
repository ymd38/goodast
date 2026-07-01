package site

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeHTTP struct {
	resp *http.Response
	err  error
}

func (f fakeHTTP) Do(*http.Request) (*http.Response, error) { return f.resp, f.err }

type fakeDNS struct {
	records []string
	err     error
}

func (f fakeDNS) LookupTXT(context.Context, string) ([]string, error) { return f.records, f.err }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func httpResp(status int, body io.Reader) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(body)}
}

const (
	tokenA = "0123456789abcdef0123456789abcdef"
	tokenB = "ffffffffffffffffffffffffffffffff"
)

func mustToken(t *testing.T, s string) VerifyToken {
	t.Helper()
	tok, err := ParseVerifyToken(s)
	if err != nil {
		t.Fatalf("ParseVerifyToken(%q): %v", s, err)
	}
	return tok
}

func TestVerifyFile(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		http    fakeHTTP
		wantErr bool
	}{
		{"success", "https://example.com", fakeHTTP{resp: httpResp(200, strings.NewReader(tokenA))}, false},
		{"success with whitespace", "https://example.com", fakeHTTP{resp: httpResp(200, strings.NewReader(tokenA + "\n"))}, false},
		{"non-200", "https://example.com", fakeHTTP{resp: httpResp(404, strings.NewReader(""))}, true},
		{"http error", "https://example.com", fakeHTTP{err: errors.New("dial fail")}, true},
		{"body read error", "https://example.com", fakeHTTP{resp: httpResp(200, errReader{})}, true},
		{"invalid content", "https://example.com", fakeHTTP{resp: httpResp(200, strings.NewReader("not-a-token"))}, true},
		{"token mismatch", "https://example.com", fakeHTTP{resp: httpResp(200, strings.NewReader(tokenB))}, true},
		{"unparseable base url", "http://%zz", fakeHTTP{resp: httpResp(200, strings.NewReader(tokenA))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewVerifier(tt.http, fakeDNS{})
			err := v.Verify(context.Background(), VerifyMethodFile, tt.baseURL, mustToken(t, tokenA))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Verify(file) err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyDNS(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		dns     fakeDNS
		wantErr bool
	}{
		{"success", "https://example.com", fakeDNS{records: []string{"unrelated", "goodast-verify=" + tokenA}}, false},
		{"no match", "https://example.com", fakeDNS{records: []string{"other=1"}}, true},
		{"lookup error", "https://example.com", fakeDNS{err: errors.New("nxdomain")}, true},
		{"unparseable base url", "http://%zz", fakeDNS{records: []string{"goodast-verify=" + tokenA}}, true},
		{"missing host", "http://", fakeDNS{records: []string{"goodast-verify=" + tokenA}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewVerifier(fakeHTTP{}, tt.dns)
			err := v.Verify(context.Background(), VerifyMethodDNSTXT, tt.baseURL, mustToken(t, tokenA))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Verify(dns) err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyUnsupportedMethod(t *testing.T) {
	v := NewVerifier(fakeHTTP{}, fakeDNS{})
	if err := v.Verify(context.Background(), VerifyMethod("bogus"), "https://example.com", mustToken(t, tokenA)); err == nil {
		t.Fatal("expected error for unsupported method")
	}
}

func TestDefaultVerifier(t *testing.T) {
	if DefaultVerifier() == nil {
		t.Fatal("DefaultVerifier returned nil")
	}
}
