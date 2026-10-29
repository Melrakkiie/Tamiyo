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
	historyReader
}

type Service struct {
	decks  deckReader
	store  store
	combos comboFinder
}

func NewService(decks deckReader, store store, combos comboFinder) *Service {
	return &Service{decks: decks, store: store, combos: combos}
}

// GetDeckLegality checks the deck viewerID reads within reach. Shared reach
// blanks the card ids of the issues.
func (s *Service) GetDeckLegality(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (LegalityReport, error) {
	read, err := s.readDeck(ctx, viewerID, deckID, reach)
	if err != nil {
		return LegalityReport{}, err
	}
	printings, err := s.store.Printings(ctx, scryfallIDs(read.Entries))
	if err != nil {
		return LegalityReport{}, fmt.Errorf("loading printings: %w", err)
	}
	report, err := checkLegality(read, printings)
	if err != nil {
		return LegalityReport{}, err
	}
	if reach == deck.ReachShared {
		report = report.withoutCardIDs()
	}
	return report, nil
}

// GetDeckStats computes the statistics of the deck viewerID reads within
// reach. Values are the owner's, whoever is viewing.
func (s *Service) GetDeckStats(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (DeckStats, error) {
	read, printings, err := s.loadMainBoard(ctx, viewerID, deckID, reach)
	if err != nil {
		return DeckStats{}, err
	}
	values, err := s.store.DeckValues(ctx, read.OwnerID, read.Deck.ID)
	if err != nil {
		return DeckStats{}, fmt.Errorf("loading deck values: %w", err)
	}
	stats := computeStats(read.MainBoard(), printings)
	stats.Value = roundToCents(values.Owned.Plus(values.Pending))
	stats.OwnedValue = roundToCents(values.Owned)
	stats.PendingValue = roundToCents(values.Pending)
	stats.BoardValues = BoardValues{
		Sideboard:   roundToCents(values.Sideboard),
		Considering: roundToCents(values.Considering),
	}
	return stats, nil
}

func (s *Service) loadMainBoard(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (deck.ReadDeck, map[string]Printing, error) {
	read, err := s.readDeck(ctx, viewerID, deckID, reach)
	if err != nil {
		return deck.ReadDeck{}, nil, err
	}
	printings, err := s.store.Printings(ctx, scryfallIDs(read.MainBoard()))
	if err != nil {
		return deck.ReadDeck{}, nil, fmt.Errorf("loading printings: %w", err)
	}
	return read, printings, nil
}

func (s *Service) readDeck(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (deck.ReadDeck, error) {
	read, err := s.decks.ReadDeck(ctx, viewerID, deckID, reach)
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
