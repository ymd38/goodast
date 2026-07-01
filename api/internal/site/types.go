// Package site はサイト登録とドメイン所有確認（ADR-0004）を担う feature パッケージ。
package site

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// VerifyMethod はドメイン所有確認の方式（ADR-0004）。
type VerifyMethod string

const (
	// VerifyMethodFile は /.well-known/goodast-verify/<token> をHTTP取得して確認する（初心者向け）。
	VerifyMethodFile VerifyMethod = "file"
	// VerifyMethodDNSTXT は TXT レコード goodast-verify=<token> で確認する（エンジニア向け）。
	VerifyMethodDNSTXT VerifyMethod = "dns-txt"
)

// ParseVerifyMethod は文字列を VerifyMethod に変換する。未知値は拒否する。
// 値は migrations の sites.verify_method CHECK 制約（'file' | 'dns-txt'）と一致させる。
func ParseVerifyMethod(s string) (VerifyMethod, error) {
	switch VerifyMethod(s) {
	case VerifyMethodFile, VerifyMethodDNSTXT:
		return VerifyMethod(s), nil
	default:
		return "", fmt.Errorf("invalid verify method %q", s)
	}
}

// verifyTokenBytes は所有確認トークンのバイト長（hex で 2 倍の文字数になる）。
const verifyTokenBytes = 16

// randRead は乱数読み取り。テストでエラー経路を検証するため差し替え可能にする。
var randRead = rand.Read

// VerifyToken はドメイン所有確認トークン。生成時にランダム性を保証し、照合はメソッドで行う。
// 認証情報（Cookie/Bearer）とは異なり秘匿対象ではない（ユーザーが自サーバに設置する公開値）。
type VerifyToken struct {
	v string
}

// NewVerifyToken は暗号論的乱数から新しいトークンを生成する。
func NewVerifyToken() (VerifyToken, error) {
	b := make([]byte, verifyTokenBytes)
	if _, err := randRead(b); err != nil {
		return VerifyToken{}, fmt.Errorf("generate verify token: %w", err)
	}
	return VerifyToken{v: hex.EncodeToString(b)}, nil
}

// ParseVerifyToken は保存済み文字列を VerifyToken に復元する。形式（hex・長さ）を検証する。
func ParseVerifyToken(s string) (VerifyToken, error) {
	if len(s) != verifyTokenBytes*2 {
		return VerifyToken{}, fmt.Errorf("invalid verify token length: %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return VerifyToken{}, fmt.Errorf("invalid verify token: %w", err)
	}
	return VerifyToken{v: s}, nil
}

// String はトークン文字列を返す。
func (t VerifyToken) String() string { return t.v }

// Equal は2つのトークンが一致するかを返す。
func (t VerifyToken) Equal(other VerifyToken) bool { return t.v == other.v }

// Site はサイトのドメイン表現（sqlc の永続化 struct と API 境界を分離する）。
// VerifyMethod / VerifyToken はローカル対象（確認不要）では nil になる。
type Site struct {
	ID                uuid.UUID
	Name              string
	BaseURL           string
	OwnershipVerified bool
	VerifyMethod      *VerifyMethod
	VerifyToken       *VerifyToken
	CreatedAt         time.Time
}
