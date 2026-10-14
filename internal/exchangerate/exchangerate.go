package exchangerate

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const (
	USD             = "usd"
	refreshInterval = 6 * time.Hour
)

type Rate struct {
	Currency string
	PerEUR   float64
	Date     time.Time
}

type Repository interface {
	Find(ctx context.Context, currency string) (Rate, bool, error)
	Save(ctx context.Context, rate Rate) error
}

type Source interface {
	Fetch(ctx context.Context, currency string) (Rate, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Current(ctx context.Context) (Rate, bool, error) {
	return s.repo.Find(ctx, USD)
}

func (s *Service) USDPerEUR(ctx context.Context) (float64, error) {
	rate, found, err := s.repo.Find(ctx, USD)
	if err != nil || !found {
		return 0, err
	}
	return rate.PerEUR, nil
}

type Refresher struct {
	repo   Repository
	source Source
	logger *zap.Logger
}

func NewRefresher(repo Repository, source Source, logger *zap.Logger) *Refresher {
	return &Refresher{repo: repo, source: source, logger: logger}
}

func (r *Refresher) Refresh(ctx context.Context) error {
	rate, err := r.source.Fetch(ctx, USD)
	if err != nil {
		return err
	}
	return r.repo.Save(ctx, rate)
}

func (r *Refresher) Run(ctx context.Context) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		if err := r.Refresh(ctx); err != nil && ctx.Err() == nil {
			r.logger.Warn("refreshing exchange rates failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
