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

func (s *Service) GetCard(ctx context.Context, id int) (Card, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) CreateCard(ctx context.Context, c Card) (Card, error) {
	return s.repo.Create(ctx, c)
}

func (s *Service) UpdateCard(ctx context.Context, id int, req updateCardRequest) (Card, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Card{}, err
	}

	updated := req.applyTo(existing)

	return s.repo.Update(ctx, updated)
}

func (s *Service) DeleteCard(ctx context.Context, id int) error {
	return s.repo.Delete(ctx, id)
}
