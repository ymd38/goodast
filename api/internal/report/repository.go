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
		var counts SeverityCounts
		if err := json.Unmarshal(row.SummaryJson, &counts); err != nil {
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

// scanDate は時系列上の点の日時を返す。完了時刻（finished_at）を優先し、
// 未設定なら作成時刻（created_at）にフォールバックする。
func scanDate(row db.ListDoneScanSummariesRow) time.Time {
	if row.FinishedAt.Valid {
		return row.FinishedAt.Time
	}
	return row.CreatedAt.Time
}

// pgUUID は uuid.UUID を pgtype.UUID に変換する（クエリ引数用）。
func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
