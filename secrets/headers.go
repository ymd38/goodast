// Package secrets は認証情報（持ち込みセッションの Cookie / Authorization ヘッダ）を
// アプリケーションレイヤーで暗号化する（ADR-0003）。api が暗号化して保存し、worker が
// 復号してスキャンに注入する。両モジュールが本パッケージを共有し、暗号化フォーマットの
// ドリフト（＝復号不能）を構造的に排除する。依存ゼロ（stdlib のみ）。
package secrets

import (
	"errors"
	"fmt"
	"strings"
)

// ヘッダ検証のドメインエラー。
var (
	// ErrNoHeaders は空のヘッダ集合を暗号化しようとした。
	ErrNoHeaders = errors.New("secrets: no headers provided")
	// ErrInvalidHeader はヘッダ名/値が不正（空・制御文字・CR/LF）。
	ErrInvalidHeader = errors.New("secrets: invalid header")
)

// Header は持ち込みセッションを構成する HTTP ヘッダ 1 個（例: Cookie / Authorization）。
// Value は機微情報のため、ログ・レスポンスに生値を出してはならない（ADR-0003）。
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Headers は持ち込みセッションを表す HTTP ヘッダ集合。
type Headers []Header

// Validate はヘッダの不正を拒否する。空集合・空名・ヘッダ名の不正文字（空白/コロン/CR/LF）・
// 値の CR/LF（ヘッダインジェクション防止）を弾く。
func (h Headers) Validate() error {
	if len(h) == 0 {
		return ErrNoHeaders
	}
	for _, hdr := range h {
		if hdr.Name == "" {
			return fmt.Errorf("%w: empty name", ErrInvalidHeader)
		}
		// HTTP フィールド名に空白・コロン・改行は現れない。値の CR/LF はインジェクション経路。
		if strings.ContainsAny(hdr.Name, " \t\r\n:") {
			return fmt.Errorf("%w: illegal char in name %q", ErrInvalidHeader, hdr.Name)
		}
		if strings.ContainsAny(hdr.Value, "\r\n") {
			return fmt.Errorf("%w: CR/LF in value of %q", ErrInvalidHeader, hdr.Name)
		}
	}
	return nil
}

// ToNucleiFormat は nuclei の WithHeaders 用に "Name: Value" 文字列スライスへ変換する。
// 入力は検証済みである前提（SealHeaders 入口 / OpenHeaders 出口の両境界で Validate 済み）。
// 純粋変換に留め、呼び出し側にエラー処理を強いない。
func (h Headers) ToNucleiFormat() []string {
	out := make([]string, 0, len(h))
	for _, hdr := range h {
		out = append(out, hdr.Name+": "+hdr.Value)
	}
	return out
}

// String は値を伏せてヘッダ名のみを返す（平文の値をログに出さない）。
func (h Headers) String() string {
	names := make([]string, len(h))
	for i, hdr := range h {
		names[i] = hdr.Name
	}
	return fmt.Sprintf("Headers(%d: %s)", len(h), strings.Join(names, ","))
}

// EncryptedHeaders は暗号化済み認証ヘッダ（nonce || ciphertext+tag）。DB の bytea に対応する。
// String() は必ずマスクし、暗号文バイト列すらログに出さない（ADR-0003）。
type EncryptedHeaders []byte

// Bytes は DB 保存用の生バイト列を返す。
func (e EncryptedHeaders) Bytes() []byte { return e }

// String はマスクした要約を返す（中身を出さない）。
func (e EncryptedHeaders) String() string {
	return fmt.Sprintf("EncryptedHeaders(%d bytes)", len(e))
}
