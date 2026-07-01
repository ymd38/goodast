package site

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ymd38/goodast/api/internal/db"
)

// Repository は sites テーブルへの永続化を担い、sqlc row ↔ ドメイン Site の変換境界となる。
type Repository struct {
	q *db.Queries
}

// NewRepository は Repository を生成する。
func NewRepository(q *db.Queries) *Repository {
	return &Repository{q: q}
}

// CreateParams はサイト作成の入力。Method / Token はローカル対象では nil。
type CreateParams struct {
	Name    string
	BaseURL string
	Method  *VerifyMethod
	Token   *VerifyToken
}

// Create はサイトを作成して返す。
func (r *Repository) Create(ctx context.Context, p CreateParams) (Site, error) {
	row, err := r.q.CreateSite(ctx, db.CreateSiteParams{
		Name:         p.Name,
		BaseUrl:      p.BaseURL,
		VerifyMethod: methodText(p.Method),
		VerifyToken:  tokenText(p.Token),
	})
	if err != nil {
		return Site{}, err
	}
	return toDomain(row)
}

// GetByID は ID でサイトを取得する。
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Site, error) {
	row, err := r.q.GetSiteByID(ctx, pgUUID(id))
	if err != nil {
		return Site{}, err
	}
	return toDomain(row)
}

// List は全サイトを新しい順に返す。
func (r *Repository) List(ctx context.Context) ([]Site, error) {
	rows, err := r.q.ListSites(ctx)
	if err != nil {
		return nil, err
	}
	sites := make([]Site, 0, len(rows))
	for _, row := range rows {
		s, err := toDomain(row)
		if err != nil {
			return nil, err
		}
		sites = append(sites, s)
	}
	return sites, nil
}

// MarkVerified はサイトを所有確認済みにして返す。
func (r *Repository) MarkVerified(ctx context.Context, id uuid.UUID) (Site, error) {
	row, err := r.q.MarkSiteVerified(ctx, pgUUID(id))
	if err != nil {
		return Site{}, err
	}
	return toDomain(row)
}

// toDomain は sqlc の Site row をドメイン Site に変換する。永続化された verify_method /
// verify_token が不正な場合はデータ不整合として error にする。
func toDomain(row db.Site) (Site, error) {
	s := Site{
		ID:                uuid.UUID(row.ID.Bytes),
		Name:              row.Name,
		BaseURL:           row.BaseUrl,
		OwnershipVerified: row.OwnershipVerified,
		CreatedAt:         row.CreatedAt.Time,
	}
	if row.VerifyMethod.Valid {
		m, err := ParseVerifyMethod(row.VerifyMethod.String)
		if err != nil {
			return Site{}, fmt.Errorf("site %s verify_method: %w", s.ID, err)
		}
		s.VerifyMethod = &m
	}
	if row.VerifyToken.Valid {
		tok, err := ParseVerifyToken(row.VerifyToken.String)
		if err != nil {
			return Site{}, fmt.Errorf("site %s verify_token: %w", s.ID, err)
		}
		s.VerifyToken = &tok
	}
	return s, nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func methodText(m *VerifyMethod) pgtype.Text {
	if m == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*m), Valid: true}
}

func tokenText(t *VerifyToken) pgtype.Text {
	if t == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: t.String(), Valid: true}
}
