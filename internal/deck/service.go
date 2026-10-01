package deck

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllDecks(ctx context.Context, userID string, filter Filter) ([]Deck, error) {
	return s.repo.FindAll(ctx, userID, filter)
}

func (s *Service) GetDeck(ctx context.Context, userID string, id int) (Deck, error) {
	return s.repo.FindByID(ctx, userID, id)
}

func (s *Service) CreateDeck(ctx context.Context, userID string, d Deck) (Deck, error) {
	return s.repo.Create(ctx, userID, d)
}

func (s *Service) UpdateDeck(ctx context.Context, userID string, id int, req updateDeckRequest) (Deck, error) {
	existing, err := s.repo.FindByID(ctx, userID, id)
	if err != nil {
		return Deck{}, err
	}

	updated := req.applyTo(existing)

	return s.repo.Update(ctx, userID, updated)
}

func (s *Service) DeleteDeck(ctx context.Context, userID string, id int) error {
	return s.repo.Delete(ctx, userID, id)
}

func (s *Service) GetDeckCards(ctx context.Context, userID string, id int) ([]DeckCard, error) {
	if _, err := s.repo.FindByID(ctx, userID, id); err != nil {
		return nil, err
	}
	return s.repo.FindCardsByDeckID(ctx, userID, id)
}

func (s *Service) PutCardInDeck(ctx context.Context, userID string, deckID, cardID int) error {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return err
	}
	return s.repo.LinkCardToDeck(ctx, userID, deckID, cardID)
}

func (s *Service) RemoveCardFromDeck(ctx context.Context, userID string, deckID, cardID int) error {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return err
	}
	return s.repo.UnlinkCardFromDeck(ctx, userID, deckID, cardID)
}
