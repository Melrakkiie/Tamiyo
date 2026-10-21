package deckinsights

import (
	"context"
	"fmt"

	"Melrakkiie/Tamiyo/internal/commanderspellbook"
)

// comboFinder looks a deck's combos up on Commander Spellbook (ADR 0010).
type comboFinder interface {
	FindCombos(ctx context.Context, commanders []string, main []string) ([]commanderspellbook.Combo, error)
}

// Combo is a set of cards in the deck that together produce a notable result,
// as listed by Commander Spellbook.
type Combo struct {
	ID      string   `json:"id"`
	Cards   []string `json:"cards"`
	Results []string `json:"results"`
	URL     string   `json:"url"`
}

// DeckCombos is the combos a deck holds.
type DeckCombos struct {
	Combos []Combo `json:"combos"`
}

// GetDeckCombos returns the combos the user's deck holds.
func (s *Service) GetDeckCombos(ctx context.Context, userID string, deckID string) (DeckCombos, error) {
	content, err := s.loadDeck(ctx, userID, deckID)
	if err != nil {
		return DeckCombos{}, err
	}
	main := mainBoard(content.Entries)
	printings, err := s.store.Printings(ctx, scryfallIDs(main))
	if err != nil {
		return DeckCombos{}, fmt.Errorf("loading printings: %w", err)
	}

	var commanderNames, mainNames []string
	for _, e := range main {
		p, ok := printings[e.ScryfallID]
		if !ok || p.Name == "" {
			continue
		}
		if e.IsCommander() {
			commanderNames = append(commanderNames, p.Name)
		} else {
			mainNames = append(mainNames, p.Name)
		}
	}

	_, err = s.combos.FindCombos(ctx, commanderNames, mainNames)
	if err != nil {
		return DeckCombos{}, err
	}
	return DeckCombos{}, nil
}
