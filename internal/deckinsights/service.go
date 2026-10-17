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

type exchangeRates interface {
	USDPerEUR(ctx context.Context) (float64, error)
}

type Service struct {
	decks    deckService
	scryfall scryfallFetcher
	rates    exchangeRates
}

func NewService(decks deckService, scryfall scryfallFetcher, rates exchangeRates) *Service {
	return &Service{decks: decks, scryfall: scryfall, rates: rates}
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
	usdPerEUR, err := s.rates.USDPerEUR(ctx)
	if err != nil {
		return DeckStats{}, fmt.Errorf("loading exchange rate: %w", err)
	}
	entries := content.Entries
	stats := computeStats(mainBoard(entries), scryfallByID, usdPerEUR)
	stats.BoardValues = BoardValues{
		Sideboard:   entriesValue(inBoard(entries, deck.BoardSideboard), scryfallByID, usdPerEUR),
		Considering: entriesValue(inBoard(entries, deck.BoardConsidering), scryfallByID, usdPerEUR),
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
