// Job contract module — api（enqueue 側）と worker（処理側）が共有する。
// 依存ゼロ（river も import しない）。ADR-0001 の Nuclei 隔離は崩さない。
module github.com/ymd38/goodast/jobs

go 1.26.4
