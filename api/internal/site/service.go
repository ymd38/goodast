package site

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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
	// ErrSelfScanForbidden は対象が GOODAST 自身の origin（ドメイン+ポート）で、
	// 自己スキャン防止のため登録できないことを表す。
	ErrSelfScanForbidden = errors.New("site: cannot register goodast's own origin")
	// ErrSiteOriginTaken は同一 origin（ドメイン+ポート）のサイトが既に登録済みであることを表す。
	// 既存サイトの ID を伴う場合は OriginTakenError を使う。
	ErrSiteOriginTaken = errors.New("site: origin already registered")
)

// OriginTakenError は同一 origin の重複登録を、既存サイト ID 付きで表す。
// handler はこの ID を 409 レスポンスに載せ、UI が既存サイトへ誘導できるようにする（履歴一元化）。
type OriginTakenError struct {
	Origin     string
	ExistingID uuid.UUID
}

func (e *OriginTakenError) Error() string {
	return fmt.Sprintf("site: origin %q already registered (site %s)", e.Origin, e.ExistingID)
}

// Is は errors.Is(err, ErrSiteOriginTaken) を真にする。
func (e *OriginTakenError) Is(target error) bool { return target == ErrSiteOriginTaken }

// uniqueViolation は PostgreSQL の一意制約違反コード。
const uniqueViolation = "23505"

// Service はサイト登録・所有確認のビジネスロジック（gin 非依存）。
type Service struct {
	repo        *Repository
	verifier    *Verifier
	selfOrigins target.SelfOrigins
	logger      *slog.Logger
}

// ServiceDeps は Service の依存（dig struct-based injection）。
type ServiceDeps struct {
	dig.In
	Repo        *Repository
	Verifier    *Verifier
	SelfOrigins target.SelfOrigins
	Logger      *slog.Logger
}

// NewService は site サービスを生成する。
func NewService(deps ServiceDeps) *Service {
	return &Service{repo: deps.Repo, verifier: deps.Verifier, selfOrigins: deps.SelfOrigins, logger: deps.Logger}
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
	// base_url を1回のパースで検証し、origin と所有確認要否を得る。
	// scheme/host 不正はクライアント入力エラーとして分類する。
	origin, required, err := target.Classify(p.BaseURL)
	if err != nil {
		return Site{}, fmt.Errorf("%w: %v", ErrInvalidBaseURL, err)
	}

	// 自己スキャン防止: GOODAST 自身の origin（ドメイン+ポート）は登録できない。
	if s.selfOrigins.Blocks(origin) {
		return Site{}, ErrSelfScanForbidden
	}

	// ローカル対象（localhost 等・ADR-0004）は確認不要。設計意図「確認スキップ即 verified」に合わせ、
	// INSERT の時点で ownership_verified を立てる（UI が別途 POST /verify を呼ばずにスキャンへ進める）。
	// INSERT 1回で確定させ、Create 成功後に別途 MarkVerified する2回書き込み（部分登録状態）を避ける。
	cp := CreateParams{Name: p.Name, BaseURL: p.BaseURL, Origin: origin, Verified: !required}
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
		if taken, mapErr := s.mapUniqueViolation(ctx, err, origin); mapErr != nil {
			return Site{}, mapErr
		} else if taken {
			// 想定外（uniqueViolation だがどの制約か特定不能）は内部エラーに倒す。
			return Site{}, fmt.Errorf("create site: %w", err)
		}
		return Site{}, fmt.Errorf("create site: %w", err)
	}
	s.logger.Info("site registered", "site_id", site.ID, "ownership_required", required)
	return site, nil
}

// mapUniqueViolation は Create の一意制約違反を、どの制約かに応じたドメインエラーに翻訳する。
//   - 戻り値 (false, nil): 一意制約違反ではない（呼び出し側で通常処理）
//   - 戻り値 (true, nil):  一意制約違反だが制約を特定できなかった（呼び出し側で内部エラー扱い）
//   - 戻り値 (_, err!=nil): 翻訳済みドメインエラー（name / origin 重複）
func (s *Service) mapUniqueViolation(ctx context.Context, createErr error, origin string) (bool, error) {
	var pgErr *pgconn.PgError
	if !errors.As(createErr, &pgErr) || pgErr.Code != uniqueViolation {
		return false, nil
	}
	switch {
	case strings.Contains(pgErr.ConstraintName, "origin"):
		// 既存サイト ID を引いて 409 に載せる（履歴一元化のため UI が誘導できる）。
		if existing, err := s.repo.GetByOrigin(ctx, origin); err == nil {
			return false, &OriginTakenError{Origin: origin, ExistingID: existing.ID}
		}
		// 既存取得に失敗しても重複自体は確定しているので ID 無しで返す。
		return false, ErrSiteOriginTaken
	case strings.Contains(pgErr.ConstraintName, "name"):
		return false, ErrSiteNameTaken
	default:
		return true, nil
	}
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
