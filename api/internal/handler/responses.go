package handler

import "github.com/ymd38/goodast/api/internal/report"

// ErrorResponse は API 共通のエラー応答（`{"error": "..."}`）。OpenAPI の @Failure で参照する。
type ErrorResponse struct {
	Error string `json:"error" example:"invalid site id"`
}

// findingsResponse は GET /scans/:id/findings の応答（明細を重大度順で内包）。
type findingsResponse struct {
	ScanID   string           `json:"scan_id"`
	Findings []report.Finding `json:"findings"`
}

// scanHistoryResponse は GET /sites/:id/scans の応答（診断履歴を新しい順で内包）。
type scanHistoryResponse struct {
	SiteID string             `json:"site_id"`
	Scans  []report.ScanState `json:"scans"`
}
