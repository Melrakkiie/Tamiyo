package card

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllCards(ctx context.Context, userID string, filter CardFilter) ([]Card, int, error) {
	return s.repo.FindAll(ctx, userID, filter)
}

func (s *Service) GetCard(ctx context.Context, userID string, id int) (Card, error) {
	return s.repo.FindByID(ctx, userID, id)
}

func (s *Service) CreateCard(ctx context.Context, userID string, c Card) (Card, error) {
	return s.repo.Create(ctx, userID, c)
}

func (s *Service) UpdateCard(ctx context.Context, userID string, id int, req updateCardRequest) (Card, error) {
	existing, err := s.repo.FindByID(ctx, userID, id)
	if err != nil {
		return Card{}, err
	}

	updated := req.applyTo(existing)

	return s.repo.Update(ctx, userID, updated)
}

func (s *Service) DeleteCard(ctx context.Context, userID string, id int) error {
	return s.repo.Delete(ctx, userID, id)
}

func (s *Service) DeleteAllCards(ctx context.Context, userID string) (int, error) {
	return s.repo.DeleteAll(ctx, userID)
}

func (s *Service) GetCardsMissingDetails(ctx context.Context, userID string) ([]Card, error) {
	return s.repo.FindMissingDetails(ctx, userID)
}

func (s *Service) SetCardDetails(ctx context.Context, userID string, id int, details Details) error {
	return s.repo.SetDetails(ctx, userID, id, details)
}
