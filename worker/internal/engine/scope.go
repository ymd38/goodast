package engine

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// dangerousPathSegments は既定で除外するパスセグメント（ADR-0004 / Critical Constraints）。
// 認証後スキャンでログアウト・データ削除・管理操作を踏まないようにする。
var dangerousPathSegments = []string{"logout", "signout", "delete", "remove", "destroy", "admin"}

// Scope はスキャン対象の許可境界（allowlist）を表す値オブジェクト。
// 登録ホスト外への逸脱と危険パスへのアクセスを拒否する判定をここに集約する。
type Scope struct {
	baseURL   string
	host      string // ホスト名のみ（小文字）。所有確認・ローカル判定に使う。
	authority string // host:port（scheme 既定ポートで補完）。allowlist 判定に使う。
}

// NewScope は base URL から Scope を生成する。http(s) かつホストを持つ URL のみ許可し、
// 不正な URL では Scope を作れない（不変条件をコンストラクタで強制）。
func NewScope(baseURL string) (Scope, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return Scope{}, fmt.Errorf("parse base url %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Scope{}, fmt.Errorf("invalid base url %q: scheme must be http or https", baseURL)
	}
	host := u.Hostname()
	if host == "" {
		return Scope{}, fmt.Errorf("invalid base url %q: missing host", baseURL)
	}
	return Scope{
		baseURL:   baseURL,
		host:      strings.ToLower(host),
		authority: authorityOf(u),
	}, nil
}

// BaseURL はスキャン投入先の URL を返す。
func (s Scope) BaseURL() string { return s.baseURL }

// Host は許可ホスト名（小文字正規化済み・ポートなし）を返す。
func (s Scope) Host() string { return s.host }

// Allows は検出 URL がスコープ内か（同一 host:port かつ 危険パスでない）を判定する。
// ポートも境界として扱うため、同一ホスト名でも別ポートの URL は拒否する。
// 解析不能な URL・host:port 不一致・危険パスはすべて拒否する。
func (s Scope) Allows(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if authorityOf(u) != s.authority {
		return false
	}
	return !IsDangerousPath(u.Path)
}

// authorityOf は URL を host:port に正規化する。明示ポートが無い場合は scheme の
// 既定ポート（http=80 / https=443）で補完し、ポート省略表記の差異を吸収する。
func authorityOf(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(u.Hostname()) + ":" + port
}

// RequiresOwnershipVerification はこのスコープが所有確認を要するかを返す（ADR-0004）。
// localhost / 127.0.0.1 / ::1 / *.local はローカル開発用として確認不要。
// worker 側の defense-in-depth（api 側の受付ゲートに加えた二重化）として用いる。
func (s Scope) RequiresOwnershipVerification() bool {
	return !isLocalTarget(s.host)
}

// isLocalTarget はローカル開発用ターゲット（所有確認スキップ対象）かを判定する。
func isLocalTarget(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasSuffix(host, ".local")
}

// IsDangerousPath はパスが危険セグメント（logout/signout/delete/remove/destroy/admin）を
// 含むかを判定する。いずれかのパスセグメントに完全一致した場合に true。
func IsDangerousPath(p string) bool {
	for seg := range strings.SplitSeq(strings.ToLower(p), "/") {
		if slices.Contains(dangerousPathSegments, seg) {
			return true
		}
	}
	return false
}
