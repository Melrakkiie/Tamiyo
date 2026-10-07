package deck

import (
	"context"
	"errors"
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
	Added           time.Time
}

type PendingRepository interface {
	FindPendingCards(ctx context.Context, userID string, deckID string) ([]PendingCard, error)
	CreatePendingCard(ctx context.Context, userID string, p PendingCard) (PendingCard, error)
	DeletePendingCard(ctx context.Context, userID string, deckID string, id int) error
}

func (s *Service) GetPendingCards(ctx context.Context, userID string, deckID string) ([]PendingCard, error) {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return nil, err
	}
	return s.repo.FindPendingCards(ctx, userID, deckID)
}

func (s *Service) AddPendingCard(ctx context.Context, userID string, deckID string, p PendingCard) (PendingCard, error) {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return PendingCard{}, err
	}
	p.DeckID = deckID
	return s.repo.CreatePendingCard(ctx, userID, p)
}

func (s *Service) RemovePendingCard(ctx context.Context, userID string, deckID string, id int) error {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return err
	}
	return s.repo.DeletePendingCard(ctx, userID, deckID, id)
}
