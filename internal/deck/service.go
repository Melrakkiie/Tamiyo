package deck

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllDecks(ctx context.Context, userID string, filter Filter) ([]Deck, int, error) {
	return s.repo.FindAll(ctx, userID, filter)
}

func (s *Service) GetDeck(ctx context.Context, userID string, id int) (Deck, error) {
	return s.repo.FindByID(ctx, userID, id)
}

func (s *Service) GetSharedDeck(ctx context.Context, shareID string) (string, Deck, error) {
	return s.repo.FindShared(ctx, shareID)
}

func (s *Service) CreateDeck(ctx context.Context, userID string, d Deck) (Deck, error) {
	return s.repo.Create(ctx, userID, d)
}

func (s *Service) UpdateDeck(ctx context.Context, userID string, id int, req updateDeckRequest) (Deck, error) {
	existing, err := s.repo.FindByID(ctx, userID, id)
	if err != nil {
		return Deck{}, err
	}

	if req.CommanderPendingID != nil && !req.ClearCommanderID && req.CommanderID == nil {
		pending, err := s.repo.FindPendingCards(ctx, userID, id)
		if err != nil {
			return Deck{}, err
		}
		if !containsPendingCard(pending, *req.CommanderPendingID) {
			return Deck{}, ErrCommanderNotFound
		}
	}

	updated := req.applyTo(existing)

	return s.repo.Update(ctx, userID, updated)
}

func containsPendingCard(pending []PendingCard, id int) bool {
	for _, p := range pending {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (s *Service) PromotePendingCommander(ctx context.Context, userID string, deckID, pendingID, cardID int) error {
	d, err := s.repo.FindByID(ctx, userID, deckID)
	if err != nil {
		return err
	}
	if d.CommanderPendingID == nil || *d.CommanderPendingID != pendingID {
		return nil
	}
	d.CommanderID = &cardID
	d.CommanderPendingID = nil
	_, err = s.repo.Update(ctx, userID, d)
	return err
}

func (s *Service) DeleteDeck(ctx context.Context, userID string, id int) error {
	return s.repo.Delete(ctx, userID, id)
}

func (s *Service) GetDeckCards(ctx context.Context, userID string, id int, sortField string, sortDesc bool) ([]DeckCard, error) {
	if _, err := s.repo.FindByID(ctx, userID, id); err != nil {
		return nil, err
	}
	return s.repo.FindCardsByDeckID(ctx, userID, id, sortField, sortDesc)
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
