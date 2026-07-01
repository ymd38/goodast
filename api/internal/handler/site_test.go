package handler

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ymd38/goodast/api/internal/site"
)

func ptrMethod(m site.VerifyMethod) *site.VerifyMethod { return &m }

func mustToken(t *testing.T) *site.VerifyToken {
	t.Helper()
	tok, err := site.ParseVerifyToken("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("ParseVerifyToken: %v", err)
	}
	return &tok
}

func TestToSiteResponse(t *testing.T) {
	tok := mustToken(t)
	base := site.Site{ID: uuid.New(), Name: "s", BaseURL: "https://example.com"}

	t.Run("file method with token includes guide", func(t *testing.T) {
		s := base
		s.VerifyMethod = ptrMethod(site.VerifyMethodFile)
		s.VerifyToken = tok
		resp := toSiteResponse(s)
		if resp.VerifyMethod == nil || resp.VerifyToken == nil || resp.Verification == nil {
			t.Fatalf("expected method/token/guide present: %+v", resp)
		}
		if resp.Verification.FilePath == "" || resp.Verification.FileContent == "" {
			t.Errorf("file guide incomplete: %+v", resp.Verification)
		}
	})

	t.Run("dns method with token includes dns guide", func(t *testing.T) {
		s := base
		s.VerifyMethod = ptrMethod(site.VerifyMethodDNSTXT)
		s.VerifyToken = tok
		resp := toSiteResponse(s)
		if resp.Verification == nil || resp.Verification.DNSRecord == "" {
			t.Errorf("dns guide missing: %+v", resp.Verification)
		}
	})

	t.Run("token without method does not panic and omits guide", func(t *testing.T) {
		s := base
		s.VerifyToken = tok // VerifyMethod is nil (inconsistent data)
		resp := toSiteResponse(s)
		if resp.Verification != nil {
			t.Errorf("guide should be omitted when method is nil: %+v", resp.Verification)
		}
		if resp.VerifyToken == nil {
			t.Error("token should still be surfaced")
		}
	})

	t.Run("method without token omits guide", func(t *testing.T) {
		s := base
		s.VerifyMethod = ptrMethod(site.VerifyMethodFile)
		resp := toSiteResponse(s)
		if resp.Verification != nil {
			t.Errorf("guide should be omitted when token is nil: %+v", resp.Verification)
		}
	})

	t.Run("local site has neither", func(t *testing.T) {
		resp := toSiteResponse(base)
		if resp.VerifyMethod != nil || resp.VerifyToken != nil || resp.Verification != nil {
			t.Errorf("local site should carry no verify fields: %+v", resp)
		}
	})
}
