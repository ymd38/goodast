package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// keyLen は AES-256 の鍵長（バイト）。
const keyLen = 32

// 暗号化のエラー。
var (
	// ErrInvalidKey は鍵が base64 不正・長さ不正で Cipher を構築できない。
	ErrInvalidKey = errors.New("secrets: invalid encryption key")
	// ErrDecrypt は復号失敗（鍵/AAD 不一致・改ざん・切り詰め）。原因の詳細は秘匿する。
	ErrDecrypt = errors.New("secrets: decryption failed")
)

// Cipher は AES-256-GCM による認証付き暗号。鍵は保持するが値は公開しない。
type Cipher struct {
	aead cipher.AEAD
	rand io.Reader // nonce 生成源。既定は crypto/rand。テストで差し替え可能。
}

// NewCipher は base64（標準エンコード）された 32 バイト鍵から AES-256-GCM Cipher を構築する。
// 鍵は環境変数からのみ渡す（設定ファイル禁止・ADR-0003 / backend.md）。
func NewCipher(keyBase64 string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode: %v", ErrInvalidKey, err)
	}
	// aes.NewCipher は 16/24/32 バイト以外を弾く。AES-256 に限定するため 32 バイトも明示検証する。
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	if len(key) != keyLen {
		return nil, fmt.Errorf("%w: key must be %d bytes for AES-256 (got %d)", ErrInvalidKey, keyLen, len(key))
	}
	// 不変条件: 32 バイト鍵（AES ブロック 16B）に対する GCM 構築は失敗しないため error は無視する。
	aead, _ := cipher.NewGCM(block)
	return &Cipher{aead: aead, rand: rand.Reader}, nil
}

// SealHeaders はヘッダを検証・JSON 化して AES-256-GCM で暗号化する。
// aad（site 識別子）を追加認証データに束ね、別 site の行へ暗号文をコピーしても復号で弾く。
// 戻り値は nonce || ciphertext+tag。
func (c *Cipher) SealHeaders(h Headers, aad []byte) (EncryptedHeaders, error) {
	if err := h.Validate(); err != nil {
		return nil, err
	}
	// 不変条件: Headers は string フィールドのみで CR/LF は Validate 済み。json.Marshal は失敗しない。
	plaintext, _ := json.Marshal(h)
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(c.rand, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	// Seal は dst(=nonce) の後ろに ciphertext+tag を追記する（結果 = nonce || ct+tag）。
	return EncryptedHeaders(c.aead.Seal(nonce, nonce, plaintext, aad)), nil
}

// OpenHeaders は SealHeaders の逆変換。aad は封緘時と同一でなければ復号に失敗する。
// 鍵/AAD 不一致・改ざん・切り詰めはすべて ErrDecrypt に丸める（原因を漏らさない）。
func (c *Cipher) OpenHeaders(enc EncryptedHeaders, aad []byte) (Headers, error) {
	ns := c.aead.NonceSize()
	if len(enc) < ns {
		return nil, fmt.Errorf("%w: ciphertext too short", ErrDecrypt)
	}
	nonce, ct := enc[:ns], enc[ns:]
	plaintext, err := c.aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecrypt, err)
	}
	var h Headers
	if err := json.Unmarshal(plaintext, &h); err != nil {
		return nil, fmt.Errorf("%w: unmarshal: %v", ErrDecrypt, err)
	}
	return h, nil
}
