// Package credential はサイトの session 認証情報（持ち込み Cookie/Bearer）の受付・暗号化保管を担う。
// 暗号化は secrets モジュール、平文の値は一切ログ・レスポンスに出さない（ADR-0003）。
package credential

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ymd38/goodast/api/internal/db"
	"github.com/ymd38/goodast/secrets"
)

// ErrNoCredentials は当該 site に認証情報が未設定（＝行の不在＝auth_mode none）。
var ErrNoCredentials = errors.New("credential: none")

// Record は認証情報のメタデータ（値は含まない）。
type Record struct {
	AuthMode  string
	CreatedAt time.Time
}

// Repository は scan_credentials への永続化を担い、sqlc row ↔ ドメイン型の変換境界となる。
type Repository struct {
	q *db.Queries
}

// NewRepository は Repository を生成する。
func NewRepository(q *db.Queries) *Repository {
	return &Repository{q: q}
}

// SiteExists は site の存在を確認する。
func (r *Repository) SiteExists(ctx context.Context, siteID uuid.UUID) (bool, error) {
	_, err := r.q.GetSiteByID(ctx, pgUUID(siteID))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get site: %w", err)
	}
	return true, nil
}

// Upsert は session 認証情報を暗号化済みヘッダで作成/更新する（site ごとに1組）。
func (r *Repository) Upsert(ctx context.Context, siteID uuid.UUID, enc secrets.EncryptedHeaders) error {
	if err := r.q.UpsertScanCredentials(ctx, db.UpsertScanCredentialsParams{
		SiteID:     pgUUID(siteID),
		EncHeaders: enc.Bytes(),
	}); err != nil {
		return fmt.Errorf("upsert credentials: %w", err)
	}
	return nil
}

// Get は認証情報メタデータを返す。未設定なら ErrNoCredentials。
func (r *Repository) Get(ctx context.Context, siteID uuid.UUID) (Record, error) {
	row, err := r.q.GetScanCredentials(ctx, pgUUID(siteID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNoCredentials
	}
	if err != nil {
		return Record{}, fmt.Errorf("get credentials: %w", err)
	}
	return Record{AuthMode: row.AuthMode, CreatedAt: row.CreatedAt.Time}, nil
}

// Delete は認証情報を削除する（none 化）。不在でもエラーにしない（冪等）。
func (r *Repository) Delete(ctx context.Context, siteID uuid.UUID) error {
	if err := r.q.DeleteScanCredentials(ctx, pgUUID(siteID)); err != nil {
		return fmt.Errorf("delete credentials: %w", err)
	}
	return nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
