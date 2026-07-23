package engine

import (
	"net/url"
	"path"
	"strings"
	"time"
)

// staticAssetExtensions は診断対象外とする静的アセットの拡張子（小文字・ドット付き）。
// CSS/JS/画像/フォント/メディア等は攻撃面でなく、Nuclei でスキャンしても無駄＋ノイズ源になる。
// .json は API レスポンス（攻撃面）になり得るため除外しない。
var staticAssetExtensions = map[string]struct{}{
	".css": {}, ".js": {}, ".mjs": {}, ".map": {},
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".svg": {}, ".webp": {}, ".ico": {}, ".bmp": {},
	".woff": {}, ".woff2": {}, ".ttf": {}, ".otf": {}, ".eot": {},
	".pdf": {}, ".mp4": {}, ".webm": {}, ".mp3": {}, ".zip": {},
}

// isStaticAsset は URL パスの拡張子が静的アセット（診断対象外）かを判定する。
// クエリは無視し、パス末尾の拡張子のみを見る。解析不能な URL は false（除外しない）。
func isStaticAsset(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	_, ok := staticAssetExtensions[strings.ToLower(path.Ext(u.Path))]
	return ok
}

// CrawlPlan はクロール段の有界化パラメータ。preset から導出される。
// Enabled=false のとき crawler は呼ばれず、診断は単一 URL のまま（現状維持）。
type CrawlPlan struct {
	Enabled  bool
	MaxDepth int
	MaxURLs  int
}

// TargetsOrBase は診断対象 URL を返す。クロールで発見した Targets があればそれを、
// 無ければ Scope.BaseURL() 単一を返す（未クロール時の後方互換）。
func TargetsOrBase(req ScanRequest) []string {
	if len(req.Targets) > 0 {
		return req.Targets
	}
	return []string{req.Scope.BaseURL()}
}

// 動的タイムアウトの係数（spec §6.1・実測でチューニング可能）。
const (
	scanTimeoutBase   = 2 * time.Minute  // 単一 URL でも確保する下駄。numURLs>=0 のため事実上の下限も兼ねる
	scanTimeoutPerURL = 10 * time.Second // 発見 URL 1 本あたりの追加枠
)

// ScanTimeout は発見 URL 数から scan 段の実行枠を算出する。
// base + numURLs×perURL を ceiling で頭打ちにする。base が最小値（下限）を兼ねるため下限クランプは不要。
// ceiling は preset 別の絶対上限（= PlanFor(preset).Timeout）で、river ジョブ Timeout と一致させる（spec §6.1）。
func ScanTimeout(numURLs int, ceiling time.Duration) time.Duration {
	d := scanTimeoutBase + time.Duration(numURLs)*scanTimeoutPerURL
	if d > ceiling {
		d = ceiling
	}
	return d
}

// CrawlCollector はクロール結果の受理判定（スコープ内 GET 到達 URL の収集・重複排除・上限判定）と
// 抽出フォーム数の集計を担う純粋ロジック。安全に関わる分岐を決定的にユニットテストするため、
// SDK アダプタ（discovery/katana）から本型へ切り出す。スレッドセーフではない（呼び出し側が直列化する）。
type CrawlCollector struct {
	scope   Scope
	maxURLs int
	seen    map[string]struct{}
	urls    []string
	forms   int
}

// NewCrawlCollector は空の収集器を生成する。maxURLs<=0 は無制限。
func NewCrawlCollector(scope Scope, maxURLs int) *CrawlCollector {
	return &CrawlCollector{scope: scope, maxURLs: maxURLs, seen: make(map[string]struct{})}
}

// Offer はクロール結果 1 件（method・rawURL）を受理判定する。
//   - 非 GET/HEAD（フォームのアクション等）: 診断対象 URL に入れず、スコープ内なら forms を加算。
//   - GET/HEAD: スコープ内・非静的アセット・非重複なら診断対象 URL に追加（maxURLs 到達後は追加しない=ハード上限）。
//
// 静的アセット（css/js/画像/フォント等）はスコープ内でも診断対象にしない（攻撃面でないため）。
// 戻り値 capped は「上限に達しておりクロールを停止してよい」を表す。
func (c *CrawlCollector) Offer(method, rawURL string) (capped bool) {
	m := strings.ToUpper(method)
	if m != "" && m != "GET" && m != "HEAD" {
		if c.scope.Allows(rawURL) {
			c.forms++
		}
		return false
	}
	if !c.scope.Allows(rawURL) {
		return false
	}
	if isStaticAsset(rawURL) {
		return false // 静的アセット（css/js/画像/フォント等）は診断対象にしない
	}
	if _, dup := c.seen[rawURL]; dup {
		return false
	}
	if c.maxURLs > 0 && len(c.urls) >= c.maxURLs {
		return true // ハード上限: 追加しない
	}
	c.seen[rawURL] = struct{}{}
	c.urls = append(c.urls, rawURL)
	return c.maxURLs > 0 && len(c.urls) >= c.maxURLs
}

// Result は収集結果を返す。
func (c *CrawlCollector) Result() CrawlResult {
	return CrawlResult{URLs: c.urls, FormCount: c.forms}
}

// DangerousPathRegexes は危険パスセグメントを Katana の OutOfScope 正規表現として返す。
// scope.go の dangerousPathSegments を唯一の正とし、セグメント境界（/ または末尾・クエリ）で
// 区切る（`/administrator-guide` の admin 部分一致は弾く。IsDangerousPath と整合）。
func DangerousPathRegexes() []string {
	pats := make([]string, 0, len(dangerousPathSegments))
	for _, seg := range dangerousPathSegments {
		pats = append(pats, `(?i)/`+seg+`(/|$|\?)`)
	}
	return pats
}
