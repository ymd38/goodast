package site

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/api/internal/target"
)

// サイト登録・所有確認のドメインエラー。
var (
	// ErrSiteNotFound は指定 site が存在しない。
	ErrSiteNotFound = errors.New("site: not found")
	// ErrSiteNameTaken は同名サイトが既に存在する。
	ErrSiteNameTaken = errors.New("site: name already taken")
	// ErrInvalidBaseURL は base_url がスキーム/ホスト等の面で不正な入力エラー。
	ErrInvalidBaseURL = errors.New("site: invalid base url")
	// ErrVerificationFailed は所有確認に失敗した（ファイル未設置・TXT不一致・到達不能）。
	ErrVerificationFailed = errors.New("site: ownership verification failed")
)

// uniqueViolation は PostgreSQL の一意制約違反コード。
const uniqueViolation = "23505"

// Service はサイト登録・所有確認のビジネスロジック（gin 非依存）。
type Service struct {
	repo     *Repository
	verifier *Verifier
	logger   *slog.Logger
}

// ServiceDeps は Service の依存（dig struct-based injection）。
type ServiceDeps struct {
	dig.In
	Repo     *Repository
	Verifier *Verifier
	Logger   *slog.Logger
}

// NewService は site サービスを生成する。
func NewService(deps ServiceDeps) *Service {
	return &Service{repo: deps.Repo, verifier: deps.Verifier, logger: deps.Logger}
}

// RegisterParams はサイト登録の入力。Method はローカル対象では無視される。
type RegisterParams struct {
	Name    string
	BaseURL string
	Method  VerifyMethod
}

// Register はサイトを登録する。非ローカル対象では所有確認トークンを発行して保存する。
// ローカル対象（localhost 等・ADR-0004）は確認不要のため method / token を持たせない。
func (s *Service) Register(ctx context.Context, p RegisterParams) (Site, error) {
	required, err := target.RequiresOwnershipVerification(p.BaseURL)
	if err != nil {
		// base_url の scheme/host 不正はクライアント入力エラーとして分類する。
		return Site{}, fmt.Errorf("%w: %v", ErrInvalidBaseURL, err)
	}

	cp := CreateParams{Name: p.Name, BaseURL: p.BaseURL}
	if required {
		tok, err := NewVerifyToken()
		if err != nil {
			return Site{}, err
		}
		method := p.Method
		cp.Method = &method
		cp.Token = &tok
	}

	site, err := s.repo.Create(ctx, cp)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Site{}, ErrSiteNameTaken
		}
		return Site{}, fmt.Errorf("create site: %w", err)
	}
	s.logger.Info("site registered", "site_id", site.ID, "ownership_required", required)
	return site, nil
}

// List は登録済みサイトを新しい順に返す。
func (s *Service) List(ctx context.Context) ([]Site, error) {
	return s.repo.List(ctx)
}

// Get は ID でサイトを取得する。存在しなければ ErrSiteNotFound。
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Site, error) {
	site, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Site{}, ErrSiteNotFound
		}
		return Site{}, fmt.Errorf("get site: %w", err)
	}
	return site, nil
}

// Verify は所有確認を実行し、成功したら ownership_verified を立てて返す。
//   - 既に確認済みなら冪等に現状を返す。
//   - ローカル対象は確認不要のため即座に verified にする。
//   - 非ローカルは保存済み method / token で Verifier を実行し、失敗なら ErrVerificationFailed。
func (s *Service) Verify(ctx context.Context, id uuid.UUID) (Site, error) {
	site, err := s.Get(ctx, id)
	if err != nil {
		return Site{}, err
	}
	if site.OwnershipVerified {
		return site, nil
	}

	required, err := target.RequiresOwnershipVerification(site.BaseURL)
	if err != nil {
		return Site{}, fmt.Errorf("evaluate target: %w", err)
	}
	if !required {
		return s.repo.MarkVerified(ctx, id)
	}

	if site.VerifyMethod == nil || site.VerifyToken == nil {
		return Site{}, fmt.Errorf("site %s missing verify method/token", id)
	}
	if err := s.verifier.Verify(ctx, *site.VerifyMethod, site.BaseURL, *site.VerifyToken); err != nil {
		s.logger.Warn("ownership verification failed", "site_id", id, "err", err)
		return Site{}, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
	}
	s.logger.Info("ownership verified", "site_id", id)
	return s.repo.MarkVerified(ctx, id)
}
