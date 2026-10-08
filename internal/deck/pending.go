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
	Added           time.Time

	OwnedCopies       int
	OwnedSamePrinting int
}

type PendingRepository interface {
	FindPendingCards(ctx context.Context, userID string, deckID string) ([]PendingCard, error)
	CreatePendingCard(ctx context.Context, userID string, p PendingCard) (PendingCard, error)
	DeletePendingCard(ctx context.Context, userID string, deckID string, id int) error
	UpdatePendingQuantity(ctx context.Context, userID string, deckID string, id int, quantity int) (PendingCard, error)
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
	for _, e := range existing {
		isCommander := d.CommanderPendingID != nil && *d.CommanderPendingID == e.ID
		if !isCommander && e.Foil == p.Foil && strings.EqualFold(e.ScryfallID, p.ScryfallID) {
			return s.repo.UpdatePendingQuantity(ctx, userID, deckID, e.ID, e.Quantity+p.Quantity)
		}
	}
	p.DeckID = deckID
	return s.repo.CreatePendingCard(ctx, userID, p)
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
