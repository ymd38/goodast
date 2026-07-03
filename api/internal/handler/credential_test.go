package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ymd38/goodast/api/internal/credential"
	"github.com/ymd38/goodast/secrets"
)

func TestWriteCredentialError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &CredentialHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"site not found", credential.ErrSiteNotFound, http.StatusNotFound},
		{"no headers", secrets.ErrNoHeaders, http.StatusBadRequest},
		{"invalid header", secrets.ErrInvalidHeader, http.StatusBadRequest},
		{"wrapped invalid header", errors.Join(errors.New("outer"), secrets.ErrInvalidHeader), http.StatusBadRequest},
		{"unexpected", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			h.writeCredentialError(c, tt.err)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestToHeaders(t *testing.T) {
	in := []headerInput{{Name: "Cookie", Value: "a=b"}, {Name: "Authorization", Value: "Bearer x"}}
	got := toHeaders(in)
	if len(got) != 2 || got[0] != (secrets.Header{Name: "Cookie", Value: "a=b"}) ||
		got[1] != (secrets.Header{Name: "Authorization", Value: "Bearer x"}) {
		t.Errorf("toHeaders() = %v", got)
	}
	if len(toHeaders(nil)) != 0 {
		t.Error("toHeaders(nil) should be empty")
	}
}
