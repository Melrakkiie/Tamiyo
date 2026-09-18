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

func (s *Service) CreateDeck(ctx context.Context, d Deck) (Deck, error) {
	return s.repo.Create(ctx, d)
}

func (s *Service) UpdateDeck(ctx context.Context, id int, req updateDeckRequest) (Deck, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Deck{}, err
	}

	updated := req.applyTo(existing)

	return s.repo.Update(ctx, updated)
}

func (s *Service) DeleteDeck(ctx context.Context, id int) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) GetDeckCards(ctx context.Context, id int) ([]DeckCard, error) {
	_, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.repo.FindCardsByDeckID(ctx, id)
}

func (s *Service) PutCardInDeck(ctx context.Context, deckID int, cardID int) error {
	return s.repo.LinkCardToDeck(ctx, deckID, cardID)
}
