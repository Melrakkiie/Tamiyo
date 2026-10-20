package deckinsights

import (
	"context"
	"errors"
	"fmt"

	"Melrakkiie/Tamiyo/internal/deck"
)

type deckService interface {
	GetDeckContent(ctx context.Context, userID string, deckID string) (deck.DeckContent, error)
}

type valueReader interface {
	DeckValues(ctx context.Context, userID string, deckID string) (DeckValues, error)
}

type printingReader interface {
	Printings(ctx context.Context, scryfallIDs []string) (map[string]Printing, error)
}

type store interface {
	valueReader
	printingReader
}

type Service struct {
	decks deckService
	store store
}

func NewService(decks deckService, store store) *Service {
	return &Service{decks: decks, store: store}
}

func (s *Service) GetDeckLegality(ctx context.Context, userID string, deckID string) (LegalityReport, error) {
	content, err := s.loadDeck(ctx, userID, deckID)
	if err != nil {
		return LegalityReport{}, err
	}
	printings, err := s.store.Printings(ctx, scryfallIDs(content.Entries))
	if err != nil {
		return LegalityReport{}, fmt.Errorf("loading printings: %w", err)
	}
	return checkLegality(content.Deck, content.Entries, printings)
}

func (s *Service) GetDeckStats(ctx context.Context, userID string, deckID string) (DeckStats, error) {
	content, err := s.loadDeck(ctx, userID, deckID)
	if err != nil {
		return DeckStats{}, err
	}
	main := mainBoard(content.Entries)
	printings, err := s.store.Printings(ctx, scryfallIDs(main))
	if err != nil {
		return DeckStats{}, fmt.Errorf("loading printings: %w", err)
	}
	values, err := s.store.DeckValues(ctx, userID, deckID)
	if err != nil {
		return DeckStats{}, fmt.Errorf("loading deck values: %w", err)
	}
	stats := computeStats(main, printings)
	stats.Value = roundToCents(values.Owned.Plus(values.Pending))
	stats.OwnedValue = roundToCents(values.Owned)
	stats.PendingValue = roundToCents(values.Pending)
	stats.BoardValues = BoardValues{
		Sideboard:   roundToCents(values.Sideboard),
		Considering: roundToCents(values.Considering),
	}
	return stats, nil
}

func (s *Service) loadDeck(ctx context.Context, userID string, deckID string) (deck.DeckContent, error) {
	content, err := s.decks.GetDeckContent(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return deck.DeckContent{}, ErrDeckNotFound
		}
		return deck.DeckContent{}, fmt.Errorf("loading deck content: %w", err)
	}
	return content, nil
}

func scryfallIDs(entries []deck.DeckEntry) []string {
	seen := make(map[string]bool, len(entries))
	var ids []string
	for _, e := range entries {
		if e.ScryfallID == "" || seen[e.ScryfallID] {
			continue
		}
		seen[e.ScryfallID] = true
		ids = append(ids, e.ScryfallID)
	}
	return ids
}
