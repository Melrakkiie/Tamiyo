package card

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllCards(ctx context.Context) ([]Card, error) {
	return s.repo.FindAll(ctx)
}
