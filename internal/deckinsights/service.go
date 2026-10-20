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

type printingReader interface {
	Printings(ctx context.Context, scryfallIDs []string) (map[string]Printing, error)
}

// store is what deck insights read from the database.
type store interface {
	valueReader
	printingReader
}

type Service struct {
	decks    deckService
	scryfall scryfallFetcher
	store    store
}

func NewService(decks deckService, scryfall scryfallFetcher, store store) *Service {
	return &Service{decks: decks, scryfall: scryfall, store: store}
}

func (s *Service) GetDeckLegality(ctx context.Context, userID string, deckID string) (LegalityReport, error) {
	content, err := s.loadDeck(ctx, userID, deckID)
	if err != nil {
		return LegalityReport{}, err
	}
	identifiers := make([]scryfall.Identifier, 0, len(content.Entries))
	for _, id := range scryfallIDs(content.Entries) {
		identifiers = append(identifiers, scryfall.Identifier{ID: id})
	}
	found, err := s.scryfall.Fetch(ctx, identifiers)
	if err != nil {
		return LegalityReport{}, fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
	}
	byID := make(map[string]scryfall.Card, len(found))
	for _, c := range found {
		byID[c.ID] = c
	}
	return checkLegality(content.Deck, content.Entries, byID)
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
	stats.Value = roundToCents(Value{EUR: values.Owned.EUR + values.Pending.EUR, USD: values.Owned.USD + values.Pending.USD})
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

// scryfallIDs lists the distinct Scryfall ids of entries.
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
