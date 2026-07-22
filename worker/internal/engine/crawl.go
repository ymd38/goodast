package engine

// CrawlPlan はクロール段の有界化パラメータ。preset から導出される。
// Enabled=false のとき crawler は呼ばれず、診断は単一 URL のまま（現状維持）。
type CrawlPlan struct {
	Enabled  bool
	MaxDepth int
	MaxURLs  int
}
