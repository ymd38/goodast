// Package scan はスキャンの受付（enqueue）を担う。実行は worker に分離されている（ADR-0001）。
package scan

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ymd38/goodast/api/internal/db"
	"github.com/ymd38/goodast/jobs"
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
func (s *Service) EnqueueScan(ctx context.Context, siteID uuid.UUID) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	scan, err := db.New(tx).CreateScan(ctx, pgtype.UUID{Bytes: siteID, Valid: true})
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
