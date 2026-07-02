package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ymd38/goodast/api/internal/scan"
)

func TestWriteScanError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &ScanHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"site not found", scan.ErrSiteNotFound, http.StatusNotFound},
		{"ownership not verified", scan.ErrOwnershipNotVerified, http.StatusForbidden},
		{"wrapped domain error", errorsJoinWrap(scan.ErrOwnershipNotVerified), http.StatusForbidden},
		{"unexpected error", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			h.writeScanError(c, tt.err)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

// errorsJoinWrap は %w ラップ越しでも errors.Is 判定が効くことを検証するためのヘルパ。
func errorsJoinWrap(err error) error {
	return errors.Join(errors.New("outer"), err)
}
