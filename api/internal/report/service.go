package report

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
