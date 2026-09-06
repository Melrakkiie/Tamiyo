package card

import (
	"context"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllCards(ctx context.Context) ([]Card, error) {
	return s.repo.FindAll(ctx)
}

func (s *Service) CreateCard(ctx context.Context, c Card) (Card, error) {
	c.Added = time.Now()
	return s.repo.Create(ctx, c)
}
