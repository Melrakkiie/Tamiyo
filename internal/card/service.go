package card

import (
	"context"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllCards(ctx context.Context, storageID *int) ([]Card, error) {
	return s.repo.FindAll(ctx, storageID)
}

func (s *Service) CreateCard(ctx context.Context, c Card) (Card, error) {
	return s.repo.Create(ctx, c)
}
