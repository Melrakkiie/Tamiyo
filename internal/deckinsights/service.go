package deckinsights

import (
	"context"
	"errors"
	"fmt"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

type deckService interface {
	GetDeckContent(ctx context.Context, userID string, deckID string) (deck.DeckContent, error)
}

type scryfallFetcher interface {
	Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error)
}

type valueReader interface {
	DeckValues(ctx context.Context, userID string, deckID string) (DeckValues, error)
}

type Service struct {
	decks    deckService
	scryfall scryfallFetcher
	values   valueReader
}

func NewService(decks deckService, scryfall scryfallFetcher, values valueReader) *Service {
	return &Service{decks: decks, scryfall: scryfall, values: values}
}

func (s *Service) GetDeckLegality(ctx context.Context, userID string, deckID string) (LegalityReport, error) {
	content, scryfallByID, err := s.loadDeckWithScryfallData(ctx, userID, deckID)
	if err != nil {
		return LegalityReport{}, err
	}
	return checkLegality(content.Deck, content.Entries, scryfallByID)
}

func (s *Service) GetDeckStats(ctx context.Context, userID string, deckID string) (DeckStats, error) {
	content, scryfallByID, err := s.loadDeckWithScryfallData(ctx, userID, deckID)
	if err != nil {
		return DeckStats{}, err
	}
	values, err := s.values.DeckValues(ctx, userID, deckID)
	if err != nil {
		return DeckStats{}, fmt.Errorf("loading deck values: %w", err)
	}
	stats := computeStats(mainBoard(content.Entries), scryfallByID)
	stats.Value = roundToCents(Value{EUR: values.Owned.EUR + values.Pending.EUR, USD: values.Owned.USD + values.Pending.USD})
	stats.OwnedValue = roundToCents(values.Owned)
	stats.PendingValue = roundToCents(values.Pending)
	stats.BoardValues = BoardValues{
		Sideboard:   roundToCents(values.Sideboard),
		Considering: roundToCents(values.Considering),
	}
	return stats, nil
}

func (s *Service) loadDeckWithScryfallData(ctx context.Context, userID string, deckID string) (deck.DeckContent, map[string]scryfall.Card, error) {
	content, err := s.decks.GetDeckContent(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return deck.DeckContent{}, nil, ErrDeckNotFound
		}
		return deck.DeckContent{}, nil, fmt.Errorf("loading deck content: %w", err)
	}

	seen := make(map[string]bool, len(content.Entries))
	var identifiers []scryfall.Identifier
	for _, e := range content.Entries {
		if e.ScryfallID == "" || seen[e.ScryfallID] {
			continue
		}
		seen[e.ScryfallID] = true
		identifiers = append(identifiers, scryfall.Identifier{ID: e.ScryfallID})
	}

	found, err := s.scryfall.Fetch(ctx, identifiers)
	if err != nil {
		return deck.DeckContent{}, nil, fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
	}

	byID := make(map[string]scryfall.Card, len(found))
	for _, c := range found {
		byID[c.ID] = c
	}

	return content, byID, nil
}
