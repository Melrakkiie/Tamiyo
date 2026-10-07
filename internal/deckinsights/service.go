package deckinsights

import (
	"context"
	"errors"
	"fmt"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

type deckService interface {
	GetDeck(ctx context.Context, userID string, id int) (deck.Deck, error)
	GetDeckCards(ctx context.Context, userID string, id int, sortField string, sortDesc bool) ([]deck.DeckCard, error)
	GetPendingCards(ctx context.Context, userID string, deckID int) ([]deck.PendingCard, error)
}

type scryfallFetcher interface {
	Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error)
}

type Service struct {
	decks    deckService
	scryfall scryfallFetcher
}

func NewService(decks deckService, scryfall scryfallFetcher) *Service {
	return &Service{decks: decks, scryfall: scryfall}
}

func (s *Service) GetDeckLegality(ctx context.Context, userID string, deckID int) (LegalityReport, error) {
	d, cards, scryfallByID, err := s.loadDeckWithScryfallData(ctx, userID, deckID)
	if err != nil {
		return LegalityReport{}, err
	}
	report, err := checkLegality(d, cards, scryfallByID)
	for i := range report.Issues {
		if report.Issues[i].CardID < 0 {
			report.Issues[i].CardID = 0
		}
	}
	return report, err
}

func (s *Service) GetDeckStats(ctx context.Context, userID string, deckID int) (DeckStats, error) {
	_, cards, scryfallByID, err := s.loadDeckWithScryfallData(ctx, userID, deckID)
	if err != nil {
		return DeckStats{}, err
	}
	return computeStats(cards, scryfallByID), nil
}

func (s *Service) loadDeckWithScryfallData(ctx context.Context, userID string, deckID int) (deck.Deck, []deck.DeckCard, map[string]scryfall.Card, error) {
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return deck.Deck{}, nil, nil, ErrDeckNotFound
		}
		return deck.Deck{}, nil, nil, err
	}

	cards, err := s.decks.GetDeckCards(ctx, userID, deckID, "updated", true)
	if err != nil {
		return deck.Deck{}, nil, nil, fmt.Errorf("loading deck cards: %w", err)
	}

	pending, err := s.decks.GetPendingCards(ctx, userID, deckID)
	if err != nil {
		return deck.Deck{}, nil, nil, fmt.Errorf("loading pending cards: %w", err)
	}
	d, cards = withPendingCards(d, cards, pending)

	seen := make(map[string]bool, len(cards))
	var identifiers []scryfall.Identifier
	for _, c := range cards {
		if c.ScryfallID == "" || seen[c.ScryfallID] {
			continue
		}
		seen[c.ScryfallID] = true
		identifiers = append(identifiers, scryfall.Identifier{ID: c.ScryfallID})
	}

	found, err := s.scryfall.Fetch(ctx, identifiers)
	if err != nil {
		return deck.Deck{}, nil, nil, fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
	}

	byID := make(map[string]scryfall.Card, len(found))
	for _, c := range found {
		byID[c.ID] = c
	}

	return d, cards, byID, nil
}

const copiesPerPendingCard = 1000

func pendingCopyID(pendingID, copyIndex int) int {
	return -(pendingID*copiesPerPendingCard + copyIndex + 1)
}

func withPendingCards(d deck.Deck, cards []deck.DeckCard, pending []deck.PendingCard) (deck.Deck, []deck.DeckCard) {
	all := make([]deck.DeckCard, 0, len(cards)+len(pending))
	all = append(all, cards...)
	for _, p := range pending {
		for copyIndex := 0; copyIndex < p.Quantity; copyIndex++ {
			all = append(all, deck.DeckCard{
				ID:              pendingCopyID(p.ID, copyIndex),
				Name:            p.Name,
				ScryfallID:      p.ScryfallID,
				SetCode:         p.SetCode,
				CollectorNumber: p.CollectorNumber,
				Foil:            p.Foil,
				ManaValue:       p.ManaValue,
				Colors:          p.Colors,
				CardType:        p.CardType,
				ColorIdentity:   p.ColorIdentity,
			})
		}
	}

	if d.CommanderID == nil && d.CommanderPendingID != nil {
		commanderID := pendingCopyID(*d.CommanderPendingID, 0)
		d.CommanderID = &commanderID
	}
	return d, all
}
