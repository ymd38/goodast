// Package scan はスキャンの受付（enqueue）を担う。実行は worker に分離されている（ADR-0001）。
package scan

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ymd38/goodast/api/internal/db"
	"github.com/ymd38/goodast/jobs"
)

// スキャン受付時のドメインエラー。
var (
	// ErrSiteNotFound は指定 site が存在しない。
	ErrSiteNotFound = errors.New("scan: site not found")
	// ErrOwnershipNotVerified は所有確認が未完了のためスキャンを実行できない（ADR-0004）。
	ErrOwnershipNotVerified = errors.New("scan: site ownership not verified")
)

// Service はスキャンジョブの受付を行う。
type Service struct {
	pool  *pgxpool.Pool
	river *river.Client[pgx.Tx]
}

// NewService は scan サービスを生成する。river は insert-only クライアント。
func NewService(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *Service {
	return &Service{pool: pool, river: riverClient}
}

// EnqueueScan は scan 行（status=queued）の作成と river ジョブの投入を
// 1トランザクションで行う（atomic enqueue）。コミットされなければジョブも scan 行も残らない。
//
// ADR-0004: ドメイン所有確認が完了するまでスキャンを実行できない。
// ただし localhost / 127.0.0.1 / ::1 / *.local はローカル開発用として確認をスキップする。
func (s *Service) EnqueueScan(ctx context.Context, siteID uuid.UUID) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	// コミット後の Rollback は no-op（ErrTxClosed）。エラーは意図的に無視する。
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)
	site, err := q.GetSiteByID(ctx, pgtype.UUID{Bytes: siteID, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrSiteNotFound
		}
		return uuid.Nil, fmt.Errorf("get site: %w", err)
	}

	required, err := requiresOwnershipVerification(site.BaseUrl)
	if err != nil {
		return uuid.Nil, fmt.Errorf("evaluate ownership requirement: %w", err)
	}
	if required && !site.OwnershipVerified {
		return uuid.Nil, ErrOwnershipNotVerified
	}

	scan, err := q.CreateScan(ctx, pgtype.UUID{Bytes: siteID, Valid: true})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create scan: %w", err)
	}

	scanID := uuid.UUID(scan.ID.Bytes)
	if _, err := s.river.InsertTx(ctx, tx, jobs.ScanArgs{ScanID: scanID.String()}, nil); err != nil {
		return uuid.Nil, fmt.Errorf("enqueue scan job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return scanID, nil
}

// requiresOwnershipVerification はターゲット URL が所有確認を要するか判定する（ADR-0004）。
// 解析できない URL は安全側に倒して「要確認」とせず、エラーを返して受付を止める。
func requiresOwnershipVerification(baseURL string) (bool, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return true, fmt.Errorf("parse base url %q: %w", baseURL, err)
	}
	return !isLocalTarget(u.Hostname()), nil
}

// isLocalTarget はローカル開発用ターゲット（所有確認スキップ対象）かを判定する。
func isLocalTarget(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasSuffix(strings.ToLower(host), ".local")
}
