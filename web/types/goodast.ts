// swagger.yaml 由来の生成型（types/api.d.ts）への短縮エイリアス。
// swaggo の定義名は完全修飾名で長いため、ここで一元的に別名を付ける
import type { components } from '~/types/api'

type Schemas = components['schemas']

export type Site = Schemas['internal_handler.siteResponse']
export type ApiErrorResponse = Schemas['internal_handler.ErrorResponse']
export type DashboardData = Schemas['github_com_ymd38_goodast_api_internal_report.DashboardData']
export type HistoryEntry = Schemas['github_com_ymd38_goodast_api_internal_report.HistoryEntry']
export type LatestState = Schemas['github_com_ymd38_goodast_api_internal_report.LatestState']
export type SeverityCounts = Schemas['github_com_ymd38_goodast_api_internal_report.SeverityCounts']
export type Band = Schemas['github_com_ymd38_goodast_api_internal_report.Band']
export type SiteResponse = Schemas['internal_handler.siteResponse']
export type ScanState = Schemas['github_com_ymd38_goodast_api_internal_report.ScanState']
export type ScanSummary = Schemas['github_com_ymd38_goodast_api_internal_report.ScanSummary']
export type Finding = Schemas['github_com_ymd38_goodast_api_internal_report.Finding']
