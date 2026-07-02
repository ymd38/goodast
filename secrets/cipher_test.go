package secrets

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// key32 は 32 バイト鍵の base64（AES-256 用）。
func key32(t *testing.T) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 32))
}

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := NewCipher(key32(t))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

func TestNewCipher(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid 32 bytes", key32(t), false},
		{"invalid base64", "not*base64*", true},
		{"too short (aes rejects)", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 10)), true},
		{"16 bytes (not AES-256)", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16)), true},
		{"24 bytes (not AES-256)", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 24)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCipher(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, ErrInvalidKey) {
					t.Errorf("err = %v, want ErrInvalidKey", err)
				}
				return
			}
			if err != nil || c == nil {
				t.Fatalf("unexpected err=%v cipher=%v", err, c)
			}
		})
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	c := newTestCipher(t)
	aad := []byte("site-123")
	in := Headers{
		{Name: "Cookie", Value: "token=abc123"},
		{Name: "Authorization", Value: "Bearer xyz"},
	}

	enc, err := c.SealHeaders(in, aad)
	if err != nil {
		t.Fatalf("SealHeaders: %v", err)
	}
	// 暗号文に平文が現れないこと。
	if bytes.Contains(enc.Bytes(), []byte("token=abc123")) || bytes.Contains(enc.Bytes(), []byte("Bearer")) {
		t.Fatal("plaintext leaked into ciphertext")
	}

	out, err := c.OpenHeaders(enc, aad)
	if err != nil {
		t.Fatalf("OpenHeaders: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("round trip length mismatch: got %d want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("header[%d] = %+v, want %+v", i, out[i], in[i])
		}
	}
}

func TestSealHeadersInvalid(t *testing.T) {
	c := newTestCipher(t)
	if _, err := c.SealHeaders(Headers{}, nil); !errors.Is(err, ErrNoHeaders) {
		t.Errorf("empty headers err = %v, want ErrNoHeaders", err)
	}
}

func TestSealHeadersNonceFailure(t *testing.T) {
	c := newTestCipher(t)
	c.rand = failingReader{}
	if _, err := c.SealHeaders(Headers{{Name: "Cookie", Value: "x"}}, nil); err == nil {
		t.Fatal("expected nonce generation error, got nil")
	}
}

func TestOpenHeadersFailures(t *testing.T) {
	c := newTestCipher(t)
	aad := []byte("site-A")
	enc, err := c.SealHeaders(Headers{{Name: "Cookie", Value: "v"}}, aad)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	t.Run("wrong aad", func(t *testing.T) {
		if _, err := c.OpenHeaders(enc, []byte("site-B")); !errors.Is(err, ErrDecrypt) {
			t.Errorf("err = %v, want ErrDecrypt", err)
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		other, _ := NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
		if _, err := other.OpenHeaders(enc, aad); !errors.Is(err, ErrDecrypt) {
			t.Errorf("err = %v, want ErrDecrypt", err)
		}
	})

	t.Run("tampered ciphertext", func(t *testing.T) {
		bad := append([]byte(nil), enc.Bytes()...)
		bad[len(bad)-1] ^= 0xFF // タグ改ざん
		if _, err := c.OpenHeaders(bad, aad); !errors.Is(err, ErrDecrypt) {
			t.Errorf("err = %v, want ErrDecrypt", err)
		}
	})

	t.Run("too short", func(t *testing.T) {
		if _, err := c.OpenHeaders(EncryptedHeaders{0x00}, aad); !errors.Is(err, ErrDecrypt) {
			t.Errorf("err = %v, want ErrDecrypt", err)
		}
	})

	t.Run("valid gcm but not headers json", func(t *testing.T) {
		// 復号は成功するが JSON として Headers にならないケース（unmarshal エラー分岐）。
		nonce := bytes.Repeat([]byte{0}, c.aead.NonceSize())
		sealed := c.aead.Seal(append([]byte(nil), nonce...), nonce, []byte("42"), aad)
		if _, err := c.OpenHeaders(sealed, aad); !errors.Is(err, ErrDecrypt) {
			t.Errorf("err = %v, want ErrDecrypt", err)
		}
	})
}

func TestNonceUniqueness(t *testing.T) {
	c := newTestCipher(t)
	h := Headers{{Name: "Cookie", Value: "v"}}
	a, _ := c.SealHeaders(h, nil)
	b, _ := c.SealHeaders(h, nil)
	if bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("two encryptions produced identical ciphertext (nonce reuse)")
	}
}

// failingReader は常に読み取り失敗する io.Reader（nonce 生成失敗の注入用）。
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("rand failure") }

func TestEncryptedHeadersString(t *testing.T) {
	e := EncryptedHeaders(bytes.Repeat([]byte{0}, 20))
	if got := e.String(); !strings.Contains(got, "20 bytes") || strings.Contains(got, "\x00") {
		t.Errorf("String() = %q, want masked byte-count summary", got)
	}
}
