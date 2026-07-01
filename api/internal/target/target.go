// Package target はスキャン対象 URL に関する共有の判定ロジックを提供する（ADR-0004）。
//
// ローカル対象の判定は「所有確認をスキップしてよいか」というセキュリティ上重要な境界であり、
// scan（受付ゲート）と site（登録・確認）の双方で同一でなければならない。ロジックの二重化に
// よるドリフト（＝安全性のほころび）を防ぐため、ここに一元化する。
package target

import (
	"fmt"
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

// RequiresOwnershipVerification は base URL が所有確認を要するかを判定する（ADR-0004）。
// http(s) かつホストを持つ URL のみ受け付け、解析不能・不正なスキームはエラーにして
// 呼び出し側で受付を止められるようにする（安全側）。
func RequiresOwnershipVerification(baseURL string) (bool, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return true, fmt.Errorf("parse base url %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return true, fmt.Errorf("invalid base url %q: scheme must be http or https", baseURL)
	}
	host := u.Hostname()
	if host == "" {
		return true, fmt.Errorf("invalid base url %q: missing host", baseURL)
	}
	return !IsLocalTarget(host), nil
}
