package deckinsights

import (
	"context"
	"fmt"

	"Melrakkiie/Tamiyo/internal/deck"
)

type deckReader interface {
	ReadDeck(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (deck.ReadDeck, error)
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
	decks  deckReader
	store  store
	combos comboFinder
}

func NewService(decks deckReader, store store, combos comboFinder) *Service {
	return &Service{decks: decks, store: store, combos: combos}
}

func (s *Service) GetDeckLegality(ctx context.Context, userID string, deckID string) (LegalityReport, error) {
	read, err := s.readOwnDeck(ctx, userID, deckID)
	if err != nil {
		return LegalityReport{}, err
	}
	printings, err := s.store.Printings(ctx, scryfallIDs(read.Entries))
	if err != nil {
		return LegalityReport{}, fmt.Errorf("loading printings: %w", err)
	}
	return checkLegality(read, printings)
}

func (s *Service) GetDeckStats(ctx context.Context, userID string, deckID string) (DeckStats, error) {
	main, printings, err := s.loadMainBoard(ctx, userID, deckID)
	if err != nil {
		return DeckStats{}, err
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

func (s *Service) loadMainBoard(ctx context.Context, userID string, deckID string) ([]deck.DeckEntry, map[string]Printing, error) {
	read, err := s.readOwnDeck(ctx, userID, deckID)
	if err != nil {
		return nil, nil, err
	}
	main := read.MainBoard()
	printings, err := s.store.Printings(ctx, scryfallIDs(main))
	if err != nil {
		return nil, nil, fmt.Errorf("loading printings: %w", err)
	}
	return main, printings, nil
}

func (s *Service) readOwnDeck(ctx context.Context, userID string, deckID string) (deck.ReadDeck, error) {
	read, err := s.decks.ReadDeck(ctx, userID, deckID, deck.ReachOwn)
	if err != nil {
		return deck.ReadDeck{}, fmt.Errorf("reading deck: %w", err)
	}
	return read, nil
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
