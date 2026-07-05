package report

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ymd38/goodast/api/internal/db"
)

// Repository は scans テーブルからダッシュボード用データを読み出し、sqlc row ↔ ドメイン
// ScanPoint の変換境界となる（summary_json の復号はここが唯一の箇所）。
type Repository struct {
	q *db.Queries
}

// NewRepository は Repository を生成する。
func NewRepository(q *db.Queries) *Repository {
	return &Repository{q: q}
}

// DoneScanPoints は指定サイトの完了スキャンを日付昇順で返す。
// summary_json を SeverityCounts へデコードし、Date は完了時刻（無ければ作成時刻）を採用する。
func (r *Repository) DoneScanPoints(ctx context.Context, siteID uuid.UUID) ([]ScanPoint, error) {
	rows, err := r.q.ListDoneScanSummaries(ctx, pgUUID(siteID))
	if err != nil {
		return nil, fmt.Errorf("list done scan summaries: %w", err)
	}
	points := make([]ScanPoint, 0, len(rows))
	for _, row := range rows {
		counts, err := decodeSummaryCounts(row.SummaryJson)
		if err != nil {
			return nil, fmt.Errorf("decode summary_json (scan %s): %w", uuid.UUID(row.ID.Bytes), err)
		}
		points = append(points, ScanPoint{
			ScanID: uuid.UUID(row.ID.Bytes).String(),
			Date:   scanDate(row),
			Counts: counts,
		})
	}
	return points, nil
}

// decodeSummaryCounts は summary_json から重大度カウントを取り出す。
// worker は scanjob.scanSummary（{"findings": {...counts...}}）として書き込むため、
// カウントは "findings" キーの下にネストしている。ここがその構造と一致させる唯一の境界。
func decodeSummaryCounts(raw []byte) (SeverityCounts, error) {
	var wrapper struct {
		Findings SeverityCounts `json:"findings"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return SeverityCounts{}, err
	}
	return wrapper.Findings, nil
}

// scanDate は時系列上の点の日時を返す。完了時刻（finished_at）を優先し、
// 未設定なら作成時刻（created_at）にフォールバックする。
func scanDate(row db.ListDoneScanSummariesRow) time.Time {
	if row.FinishedAt.Valid {
		return row.FinishedAt.Time
	}
	return row.CreatedAt.Time
}

// GetScanState は scan 1 件の状態（status＋サマリ）を返す。summary_json を持つ（done）場合は
// スコアを算出して Summary に載せ、未完了なら Summary は nil にする。
// scan が存在しない場合は pgx.ErrNoRows をラップして返す（service が ErrScanNotFound へ翻訳）。
func (r *Repository) GetScanState(ctx context.Context, id uuid.UUID) (ScanState, error) {
	row, err := r.q.GetScan(ctx, pgUUID(id))
	if err != nil {
		return ScanState{}, fmt.Errorf("get scan: %w", err)
	}
	return toScanState(row)
}

// ListSiteScans はサイトの全スキャンを新しい順で返す（§6.5 診断履歴）。
// done/queued/running/failed を問わず含め、done は summary（スコア込み）を、未完了は nil を持つ。
func (r *Repository) ListSiteScans(ctx context.Context, siteID uuid.UUID) ([]ScanState, error) {
	rows, err := r.q.ListScansBySite(ctx, pgUUID(siteID))
	if err != nil {
		return nil, fmt.Errorf("list scans by site: %w", err)
	}
	scans := make([]ScanState, 0, len(rows))
	for _, row := range rows {
		state, err := toScanState(row)
		if err != nil {
			return nil, err
		}
		scans = append(scans, state)
	}
	return scans, nil
}

// toScanState は scans 行を ScanState に変換する（GetScan / ListScansBySite 共通）。
// summary_json を持つ（done）行はスコアを算出して Summary に載せ、未完了は nil にする。
func toScanState(row db.Scan) (ScanState, error) {
	state := ScanState{
		ID:            uuid.UUID(row.ID.Bytes).String(),
		SiteID:        uuid.UUID(row.SiteID.Bytes).String(),
		Status:        row.Status,
		EngineVersion: textValue(row.EngineVersion),
		CreatedAt:     row.CreatedAt.Time,
		StartedAt:     timePtr(row.StartedAt),
		FinishedAt:    timePtr(row.FinishedAt),
	}
	if len(row.SummaryJson) > 0 {
		counts, err := decodeSummaryCounts(row.SummaryJson)
		if err != nil {
			return ScanState{}, fmt.Errorf("decode summary_json (scan %s): %w", state.ID, err)
		}
		summary := buildScanSummary(counts)
		state.Summary = &summary
	}
	return state, nil
}

// ScanExists は scan 行の存在確認を返す（findings エンドポイントの 404 判定用）。
func (r *Repository) ScanExists(ctx context.Context, id uuid.UUID) (bool, error) {
	exists, err := r.q.ScanExists(ctx, pgUUID(id))
	if err != nil {
		return false, fmt.Errorf("scan exists: %w", err)
	}
	return exists, nil
}

// ScanFindings は scan の findings 明細を重大度順で返す。
func (r *Repository) ScanFindings(ctx context.Context, id uuid.UUID) ([]Finding, error) {
	rows, err := r.q.ListFindingsByScan(ctx, pgUUID(id))
	if err != nil {
		return nil, fmt.Errorf("list findings by scan: %w", err)
	}
	findings := make([]Finding, 0, len(rows))
	for _, row := range rows {
		findings = append(findings, Finding{
			ID:          uuid.UUID(row.ID.Bytes).String(),
			TemplateID:  row.TemplateID,
			Title:       row.Title,
			Severity:    row.Severity,
			URL:         row.Url,
			CWE:         textValue(row.Cwe),
			Remediation: textValue(row.Remediation),
			Status:      row.Status,
		})
	}
	return findings, nil
}

// textValue は nullable な pgtype.Text を string に変換する（NULL は ""）。
func textValue(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// timePtr は nullable な pgtype.Timestamptz を *time.Time に変換する（NULL は nil）。
func timePtr(t pgtype.Timestamptz) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}

// pgUUID は uuid.UUID を pgtype.UUID に変換する（クエリ引数用）。
func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
