// Package target はスキャン対象 URL に関する共有の判定ロジックを提供する（ADR-0004）。
//
// ローカル対象の判定は「所有確認をスキップしてよいか」というセキュリティ上重要な境界であり、
// scan（受付ゲート）と site（登録・確認）の双方で同一でなければならない。ロジックの二重化に
// よるドリフト（＝安全性のほころび）を防ぐため、ここに一元化する。
package target

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// IsLocalTarget はローカル開発用ターゲット（所有確認スキップ対象）かを判定する。
// localhost / 127.0.0.1 / ::1 / *.local が対象（ADR-0004）。
func IsLocalTarget(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasSuffix(strings.ToLower(host), ".local")
}

// parseHostPort は base URL を検証し、ホストと実効ポート（スキーム既定を補完）を返す。
// http(s) かつホストを持つ URL のみ受け付け、解析不能・不正なスキームはエラーにする（安全側）。
// RequiresOwnershipVerification / CanonicalOrigin / Classify が検証ロジックを共有する。
func parseHostPort(baseURL string) (host, port string, err error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", "", fmt.Errorf("parse base url %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", fmt.Errorf("invalid base url %q: scheme must be http or https", baseURL)
	}
	host = u.Hostname()
	if host == "" {
		return "", "", fmt.Errorf("invalid base url %q: missing host", baseURL)
	}
	port = u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port, nil
}

// RequiresOwnershipVerification は base URL が所有確認を要するかを判定する（ADR-0004）。
func RequiresOwnershipVerification(baseURL string) (bool, error) {
	host, _, err := parseHostPort(baseURL)
	if err != nil {
		return true, err
	}
	return !IsLocalTarget(host), nil
}

// canonicalHostPort はホストを小文字化し、ループバック別名（127.0.0.1 / ::1）を
// localhost に畳み込んで "host:port" を返す。同一マシン・同一ポートを同じ origin として
// 扱うための正規化。self 判定と origin 一意制約の双方がこの正規化を共有する。
func canonicalHostPort(host, port string) string {
	host = strings.ToLower(host)
	switch host {
	case "127.0.0.1", "::1":
		host = "localhost"
	}
	// net.JoinHostPort は IPv6 ホスト（コロンを含む）を自動でブラケット表記にする。
	return net.JoinHostPort(host, port)
}

// CanonicalOrigin は base URL を正規化した origin（ドメイン+ポート）を返す。
// スキーム既定ポート（https=443 / http=80）を補完し、ループバック別名を畳み込む。
// migrations 000008 の origin backfill はこの正規化に合わせている。
func CanonicalOrigin(baseURL string) (string, error) {
	host, port, err := parseHostPort(baseURL)
	if err != nil {
		return "", err
	}
	return canonicalHostPort(host, port), nil
}

// Classify は base URL を1回のパースで検証し、正規化 origin と所有確認要否を同時に返す。
// site 登録が origin 一意判定と所有確認判定の双方を必要とするため、二重パース・
// 到達不能なエラー分岐を避けてここに集約する。
func Classify(baseURL string) (origin string, requiresVerification bool, err error) {
	host, port, err := parseHostPort(baseURL)
	if err != nil {
		return "", true, err
	}
	return canonicalHostPort(host, port), !IsLocalTarget(host), nil
}

// SelfOrigins は GOODAST 自身の origin 集合（正規化済み）。ここに一致する対象は
// 自己スキャン防止のため登録を拒否する。
type SelfOrigins map[string]bool

// NewSelfOrigins は "host:port" 形式のエントリ列から正規化済みの SelfOrigins を構築する。
// 空エントリは無視し、host/port を欠く不正エントリはエラーにする（設定ミスを起動時に弾く）。
func NewSelfOrigins(entries []string) (SelfOrigins, error) {
	set := make(SelfOrigins, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		host, port, err := net.SplitHostPort(e)
		if err != nil {
			return nil, fmt.Errorf("invalid self origin %q: %w", e, err)
		}
		if host == "" || port == "" {
			return nil, fmt.Errorf("invalid self origin %q: host and port are required", e)
		}
		set[canonicalHostPort(host, port)] = true
	}
	return set, nil
}

// Blocks は与えられた正規化 origin が GOODAST 自身に一致するかを返す。
func (s SelfOrigins) Blocks(origin string) bool {
	return s[origin]
}
