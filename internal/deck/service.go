package deck

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllDecks(ctx context.Context) ([]Deck, error) {
	return s.repo.FindAll(ctx)
}

func (s *Service) GetDeck(ctx context.Context, id int) (Deck, error) {
	return s.repo.FindByID(ctx, id)
}
