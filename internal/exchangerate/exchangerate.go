package exchangerate

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

const (
	USD             = "usd"
	refreshInterval = 6 * time.Hour
	retryInterval   = 5 * time.Minute
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

type FallbackSource []Source

func (f FallbackSource) Fetch(ctx context.Context, currency string) (Rate, error) {
	var errs []error
	for _, source := range f {
		rate, err := source.Fetch(ctx, currency)
		if err == nil {
			return rate, nil
		}
		errs = append(errs, err)
	}
	return Rate{}, errors.Join(errs...)
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
	repo            Repository
	source          Source
	logger          *zap.Logger
	refreshInterval time.Duration
	retryInterval   time.Duration
}

func NewRefresher(repo Repository, source Source, logger *zap.Logger) *Refresher {
	return &Refresher{
		repo:            repo,
		source:          source,
		logger:          logger,
		refreshInterval: refreshInterval,
		retryInterval:   retryInterval,
	}
}

func (r *Refresher) Refresh(ctx context.Context) error {
	rate, err := r.source.Fetch(ctx, USD)
	if err != nil {
		return err
	}
	if err := r.repo.Save(ctx, rate); err != nil {
		return err
	}
	r.logger.Info("exchange rate refreshed", zap.String("currency", rate.Currency),
		zap.Float64("per_eur", rate.PerEUR), zap.String("date", rate.Date.Format(time.DateOnly)))
	return nil
}

func (r *Refresher) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		delay := r.refreshInterval
		if err := r.Refresh(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			r.logger.Warn("refreshing exchange rates failed", zap.Error(err), zap.Duration("retry_in", r.retryInterval))
			delay = r.retryInterval
		}
		timer.Reset(delay)
	}
}
