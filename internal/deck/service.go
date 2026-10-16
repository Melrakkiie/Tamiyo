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

func (s *Service) GetDeck(ctx context.Context, userID string, id string) (Deck, error) {
	return s.repo.FindByID(ctx, userID, id)
}

func (s *Service) GetSharedDeck(ctx context.Context, id string) (string, Deck, error) {
	return s.repo.FindShared(ctx, id)
}

func (s *Service) CreateDeck(ctx context.Context, userID string, d Deck) (Deck, error) {
	return s.repo.Create(ctx, userID, d)
}

func (s *Service) UpdateDeck(ctx context.Context, userID string, id string, req updateDeckRequest) (Deck, error) {
	existing, err := s.repo.FindByID(ctx, userID, id)
	if err != nil {
		return Deck{}, err
	}

	for _, pendingID := range req.pendingIDs() {
		pending, err := s.repo.FindPendingCards(ctx, userID, id)
		if err != nil {
			return Deck{}, err
		}
		if !containsPendingCard(pending, pendingID) {
			return Deck{}, ErrCommanderNotFound
		}
	}

	updated := req.applyTo(existing)
	if err := updated.checkCommanders(); err != nil {
		return Deck{}, err
	}

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

func (s *Service) PromotePendingCommander(ctx context.Context, userID string, deckID string, pendingID, cardID int) error {
	d, err := s.repo.FindByID(ctx, userID, deckID)
	if err != nil {
		return err
	}
	slot, ok := d.PendingSlot(pendingID)
	if !ok {
		return nil
	}
	d.setSlot(slot, &cardID, nil)
	_, err = s.repo.Update(ctx, userID, d)
	return err
}

func (s *Service) SetCardCommander(ctx context.Context, userID string, deckID string, slot CommanderSlot, cardID int) error {
	return s.setCommander(ctx, userID, deckID, slot, &cardID, nil)
}

func (s *Service) SetPendingCommander(ctx context.Context, userID string, deckID string, slot CommanderSlot, pendingID int) error {
	return s.setCommander(ctx, userID, deckID, slot, nil, &pendingID)
}

func (s *Service) setCommander(ctx context.Context, userID string, deckID string, slot CommanderSlot, cardID, pendingID *int) error {
	d, err := s.repo.FindByID(ctx, userID, deckID)
	if err != nil {
		return err
	}
	d.setSlot(slot, cardID, pendingID)
	if err := d.checkCommanders(); err != nil {
		return err
	}
	_, err = s.repo.Update(ctx, userID, d)
	return err
}

func (s *Service) DeleteDeck(ctx context.Context, userID string, id string) error {
	return s.repo.Delete(ctx, userID, id)
}

func (s *Service) GetDeckCards(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]DeckCard, error) {
	if _, err := s.repo.FindByID(ctx, userID, id); err != nil {
		return nil, err
	}
	return s.repo.FindCardsByDeckID(ctx, userID, id, sortField, sortDesc)
}

func (s *Service) PutCardInDeck(ctx context.Context, userID string, deckID string, cardID int, board string) error {
	d, err := s.repo.FindByID(ctx, userID, deckID)
	if err != nil {
		return err
	}
	if !InMainBoard(board) && d.IsCommanderCard(cardID) {
		return ErrCommanderBoard
	}
	return s.repo.LinkCardToDeck(ctx, userID, deckID, cardID, board)
}

func (s *Service) RemoveCardFromDeck(ctx context.Context, userID string, deckID string, cardID int) error {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return err
	}
	return s.repo.UnlinkCardFromDeck(ctx, userID, deckID, cardID)
}

func (s *Service) ClearCommander(ctx context.Context, userID string, deckID string, slot CommanderSlot) error {
	d, err := s.repo.FindByID(ctx, userID, deckID)
	if err != nil {
		return err
	}
	d.clearSlot(slot)
	_, err = s.repo.Update(ctx, userID, d)
	return err
}
