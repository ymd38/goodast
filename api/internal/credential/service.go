package credential

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.uber.org/dig"

	"github.com/ymd38/goodast/secrets"
)

// ErrSiteNotFound は指定 site が存在しない。
var ErrSiteNotFound = errors.New("credential: site not found")

// Status は認証情報の状態（マスク済み・値を含まない）。
type Status struct {
	AuthMode   string // "none" | "session"
	Configured bool
	CreatedAt  *string // session のとき RFC3339 の設定日時
}

// Service は認証情報のビジネスロジック（暗号化保管）を担う。gin/net/http に依存しない。
type Service struct {
	repo   *Repository
	cipher *secrets.Cipher
	logger *slog.Logger
}

// ServiceDeps は Service の依存（dig struct-based injection）。
type ServiceDeps struct {
	dig.In
	Repo   *Repository
	Cipher *secrets.Cipher
	Logger *slog.Logger
}

// NewService は credential サービスを生成する。
func NewService(d ServiceDeps) *Service {
	return &Service{repo: d.Repo, cipher: d.Cipher, logger: d.Logger}
}

// SetSession は session 認証情報を暗号化して保存する。ヘッダは平文でログしない（ADR-0003）。
// aad に site_id を束ね、暗号文の別 site 行への流用を復号時に弾く。
func (s *Service) SetSession(ctx context.Context, siteID uuid.UUID, headers secrets.Headers) error {
	if err := s.ensureSiteExists(ctx, siteID); err != nil {
		return err
	}
	enc, err := s.cipher.SealHeaders(headers, siteID[:])
	if err != nil {
		// ErrNoHeaders / ErrInvalidHeader は呼び出し側で 400 に対応づける。
		return err
	}
	return s.repo.Upsert(ctx, siteID, enc)
}

// Clear は認証情報を削除する（none 化）。未設定でも冪等に成功する。
func (s *Service) Clear(ctx context.Context, siteID uuid.UUID) error {
	if err := s.ensureSiteExists(ctx, siteID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, siteID)
}

// GetStatus はマスク済みの認証情報状態を返す（値・ヘッダ名を含まない）。
func (s *Service) GetStatus(ctx context.Context, siteID uuid.UUID) (Status, error) {
	if err := s.ensureSiteExists(ctx, siteID); err != nil {
		return Status{}, err
	}
	rec, err := s.repo.Get(ctx, siteID)
	if errors.Is(err, ErrNoCredentials) {
		return statusNone(), nil
	}
	if err != nil {
		return Status{}, err
	}
	// 「none は行の不在」が原則だが、旧データ・外部挿入で auth_mode='none' 行が残り得る。
	// configured は session 行に限定し、レスポンスの整合性を保つ（none 行を configured 扱いしない）。
	if rec.AuthMode != "session" {
		return statusNone(), nil
	}
	created := rec.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	return Status{AuthMode: "session", Configured: true, CreatedAt: &created}, nil
}

// statusNone は認証情報未設定のマスク状態を返す。
func statusNone() Status {
	return Status{AuthMode: "none", Configured: false}
}

func (s *Service) ensureSiteExists(ctx context.Context, siteID uuid.UUID) error {
	exists, err := s.repo.SiteExists(ctx, siteID)
	if err != nil {
		return fmt.Errorf("check site: %w", err)
	}
	if !exists {
		return ErrSiteNotFound
	}
	return nil
}
