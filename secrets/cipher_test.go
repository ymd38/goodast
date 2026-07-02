package secrets

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// sealRaw は任意の平文を正規エンベロープ（version || nonce || ct+tag・version を AAD に束ねる）で
// 暗号化する白箱ヘルパ。OpenHeaders の unmarshal / 再検証の分岐を突くために使う。
func sealRaw(c *Cipher, plaintext, aad []byte) EncryptedHeaders {
	nonce := bytes.Repeat([]byte{0}, c.aead.NonceSize())
	env := append([]byte{formatVersion}, nonce...)
	return EncryptedHeaders(c.aead.Seal(env, nonce, plaintext, versionedAAD(formatVersion, aad)))
}

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

	t.Run("unsupported version", func(t *testing.T) {
		bad := append([]byte(nil), enc.Bytes()...)
		bad[0] = 0x09 // 未知バージョン
		if _, err := c.OpenHeaders(bad, aad); !errors.Is(err, ErrDecrypt) {
			t.Errorf("err = %v, want ErrDecrypt", err)
		}
	})

	t.Run("valid gcm but not headers json", func(t *testing.T) {
		// 復号は成功するが JSON として Headers にならないケース（unmarshal エラー分岐）。
		if _, err := c.OpenHeaders(sealRaw(c, []byte("42"), aad), aad); !errors.Is(err, ErrDecrypt) {
			t.Errorf("err = %v, want ErrDecrypt", err)
		}
	})

	t.Run("decrypts to invalid headers (fail closed)", func(t *testing.T) {
		// 復号・JSON パースは成功するが CR/LF を含む不正ヘッダ。trust boundary の再検証で弾く。
		pt, _ := json.Marshal(Headers{{Name: "Cookie", Value: "a\r\nInjected: 1"}})
		_, err := c.OpenHeaders(sealRaw(c, pt, aad), aad)
		if !errors.Is(err, ErrInvalidHeader) {
			t.Errorf("err = %v, want ErrInvalidHeader", err)
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
