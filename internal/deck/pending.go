package deck

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrPendingCardNotFound = errors.New("pending card not found")

type PendingCard struct {
	ID              int
	DeckID          string
	Name            string
	ScryfallID      string
	SetCode         string
	CollectorNumber string
	Foil            bool
	Quantity        int
	ManaValue       float64
	Colors          *string
	CardType        *string
	ColorIdentity   *string
	Board           string
	Added           time.Time

	OwnedCopies       int
	OwnedSamePrinting int
}

type PendingRepository interface {
	FindPendingCards(ctx context.Context, userID string, deckID string) ([]PendingCard, error)
	CreatePendingCard(ctx context.Context, userID string, p PendingCard) (PendingCard, error)
	DeletePendingCard(ctx context.Context, userID string, deckID string, id int) error
	UpdatePendingQuantity(ctx context.Context, userID string, deckID string, id int, quantity int) (PendingCard, error)
	UpdatePendingBoard(ctx context.Context, userID string, deckID string, id int, board string) (PendingCard, error)
}

func (s *Service) GetPendingCards(ctx context.Context, userID string, deckID string) ([]PendingCard, error) {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return nil, err
	}
	return s.repo.FindPendingCards(ctx, userID, deckID)
}

func (s *Service) AddPendingCard(ctx context.Context, userID string, deckID string, p PendingCard) (PendingCard, error) {
	d, err := s.repo.FindByID(ctx, userID, deckID)
	if err != nil {
		return PendingCard{}, err
	}
	existing, err := s.repo.FindPendingCards(ctx, userID, deckID)
	if err != nil {
		return PendingCard{}, err
	}
	if p.Board == "" {
		p.Board = BoardMain
	}
	if e, ok := mergeTarget(d, existing, p, 0); ok {
		return s.repo.UpdatePendingQuantity(ctx, userID, deckID, e.ID, e.Quantity+p.Quantity)
	}
	p.DeckID = deckID
	return s.repo.CreatePendingCard(ctx, userID, p)
}

func isPendingCommander(d Deck, id int) bool {
	return d.CommanderPendingID != nil && *d.CommanderPendingID == id
}

func mergeTarget(d Deck, existing []PendingCard, p PendingCard, skipID int) (PendingCard, bool) {
	for _, e := range existing {
		if e.ID == skipID || isPendingCommander(d, e.ID) {
			continue
		}
		if e.Board == p.Board && e.Foil == p.Foil && strings.EqualFold(e.ScryfallID, p.ScryfallID) {
			return e, true
		}
	}
	return PendingCard{}, false
}

func (s *Service) UpdatePendingCard(ctx context.Context, userID string, deckID string, id int, quantity *int, board *string) (PendingCard, error) {
	d, err := s.repo.FindByID(ctx, userID, deckID)
	if err != nil {
		return PendingCard{}, err
	}
	existing, err := s.repo.FindPendingCards(ctx, userID, deckID)
	if err != nil {
		return PendingCard{}, err
	}
	current, found := findPending(existing, id)
	if !found {
		return PendingCard{}, ErrPendingCardNotFound
	}
	if board != nil && *board != current.Board && isPendingCommander(d, id) {
		return PendingCard{}, ErrCommanderBoard
	}
	if quantity != nil {
		if current, err = s.repo.UpdatePendingQuantity(ctx, userID, deckID, id, *quantity); err != nil {
			return PendingCard{}, err
		}
	}
	if board == nil || *board == current.Board {
		return current, nil
	}
	moved := current
	moved.Board = *board
	if e, ok := mergeTarget(d, existing, moved, id); ok {
		merged, err := s.repo.UpdatePendingQuantity(ctx, userID, deckID, e.ID, e.Quantity+current.Quantity)
		if err != nil {
			return PendingCard{}, err
		}
		if err := s.repo.DeletePendingCard(ctx, userID, deckID, id); err != nil {
			return PendingCard{}, err
		}
		return merged, nil
	}
	return s.repo.UpdatePendingBoard(ctx, userID, deckID, id, *board)
}

func findPending(pending []PendingCard, id int) (PendingCard, bool) {
	for _, p := range pending {
		if p.ID == id {
			return p, true
		}
	}
	return PendingCard{}, false
}

func (s *Service) SetPendingQuantity(ctx context.Context, userID string, deckID string, id int, quantity int) (PendingCard, error) {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return PendingCard{}, err
	}
	return s.repo.UpdatePendingQuantity(ctx, userID, deckID, id, quantity)
}

func (s *Service) RemovePendingCard(ctx context.Context, userID string, deckID string, id int) error {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return err
	}
	return s.repo.DeletePendingCard(ctx, userID, deckID, id)
}
