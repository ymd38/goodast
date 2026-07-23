// Secrets module — 認証情報（Cookie/Bearer ヘッダ）のアプリ層暗号化を担う（ADR-0003）。
// api（暗号化して保存）と worker（復号して注入）が共有し、暗号化フォーマットのドリフトを
// 構造的に排除する。依存ゼロ（stdlib のみ）。ADR-0001 の Nuclei 隔離は崩さない。
module github.com/ymd38/goodast/secrets

go 1.26.5
