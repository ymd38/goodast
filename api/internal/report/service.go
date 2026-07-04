package report

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Service はダッシュボード集計のビジネスロジック（usecase 相当）。gin / net/http に依存しない。
type Service struct {
	repo *Repository
}

// NewService は Service を生成する。
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Dashboard は指定サイトのダッシュボードデータを集計して返す。
// done スキャンが無い（未知サイト含む）場合は Latest=nil・History=空 の DashboardData を返す。
func (s *Service) Dashboard(ctx context.Context, siteID uuid.UUID) (DashboardData, error) {
	points, err := s.repo.DoneScanPoints(ctx, siteID)
	if err != nil {
		return DashboardData{}, fmt.Errorf("load done scan points: %w", err)
	}
	return BuildDashboard(points), nil
}

// ScanState は scan の状態（レポート上段・進捗ポーリング兼用）を返す。
// scan が存在しなければ ErrScanNotFound を返す。
func (s *Service) ScanState(ctx context.Context, scanID uuid.UUID) (ScanState, error) {
	state, err := s.repo.GetScanState(ctx, scanID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScanState{}, ErrScanNotFound
		}
		return ScanState{}, fmt.Errorf("get scan state: %w", err)
	}
	return state, nil
}

// SiteScans はサイトの診断履歴（全スキャンを新しい順）を返す（§6.5）。
// スキャンが無い（未知サイト含む）場合は空スライスを返す。
func (s *Service) SiteScans(ctx context.Context, siteID uuid.UUID) ([]ScanState, error) {
	scans, err := s.repo.ListSiteScans(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("list site scans: %w", err)
	}
	return scans, nil
}

// ScanFindings は scan の findings 明細を返す。scan が存在しなければ ErrScanNotFound を返す
// （findings が 0 件なのか scan 自体が無いのかを区別する）。
func (s *Service) ScanFindings(ctx context.Context, scanID uuid.UUID) ([]Finding, error) {
	exists, err := s.repo.ScanExists(ctx, scanID)
	if err != nil {
		return nil, fmt.Errorf("check scan exists: %w", err)
	}
	if !exists {
		return nil, ErrScanNotFound
	}
	findings, err := s.repo.ScanFindings(ctx, scanID)
	if err != nil {
		return nil, fmt.Errorf("load scan findings: %w", err)
	}
	return findings, nil
}
