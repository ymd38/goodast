// Package engine はスキャンエンジンの抽象とガードレールを提供する。
//
// ADR-0002: Nuclei SDK の import は本パッケージ配下（具体的には engine/nuclei）にのみ許可する。
// 本ファイル自体は SDK 非依存の純粋なドメイン定義に留め、ユニットテストで網羅する。
// 実装（Nuclei アダプタ）は engine/nuclei に置き、//go:build integration で検証する。
package engine

import "context"

// Finding は engine が検出した脆弱性 1 件のドメイン表現（DB 永続化前）。
// 重大度は正規化済みの Severity で持ち、URL はガードレール（スコープ）通過済みのもの。
type Finding struct {
	TemplateID  string
	Title       string
	Severity    Severity
	URL         string
	CWE         string
	Remediation string
}

// FindingCallback は検出 1 件ごとに呼ばれる。並行スキャン下で複数 goroutine から
// 呼ばれ得るため、実装は goroutine-safe であること（呼び出し側の責務）。
type FindingCallback func(Finding)

// ScanRequest は 1 回のスキャン要求。対象・許可境界・実行パラメータを内包する。
type ScanRequest struct {
	// Scope はスキャン対象の許可境界（allowlist）。エンジンはこの外へ逸脱しない。
	Scope Scope
	// Targets はクロール段が発見した診断対象 URL 群。空なら Scope.BaseURL() 単一にフォールバック。
	Targets []string
	// Headers は全リクエストに付与する認証ヘッダ（"Name: Value" 形式）。
	// 認証後スキャンで持ち込みセッションを注入する（ADR-0003）。未認証時は空。
	Headers []string
	// Profile は preset 由来の実行パラメータ（テンプレート選択・レート）。
	Profile ScanProfile
}

// Engine はスキャンエンジンの抽象。
//
// ADR-0002 がインターフェース化を許容する唯一の箇所（フェーズ2で ZAP を第2実装として追加予定）。
// Scan は検出ごとに onFinding を呼び、スキャン完了で nil、失敗で error を返す。
type Engine interface {
	// Scan は対象スコープをスキャンし、検出ごとに onFinding を呼ぶ。
	Scan(ctx context.Context, req ScanRequest, onFinding FindingCallback) error
	// Version はエンジンの識別子＋固定バージョン（例: "nuclei/v3.9.0"）を返す。
	// scans.engine_version に記録し、テンプレート由来の挙動変化の追跡に用いる（ADR-0002）。
	Version() string
}
