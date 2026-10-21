package deckinsights

import (
	"context"
	"fmt"

	"Melrakkiie/Tamiyo/internal/commanderspellbook"
	"Melrakkiie/Tamiyo/internal/deck"
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

	commanders, others := comboDeckList(main, printings)
	if len(commanders) == 0 && len(others) == 0 {
		return DeckCombos{Combos: []Combo{}}, nil
	}

	_, err = s.combos.FindCombos(ctx, commanders, others)
	if err != nil {
		return DeckCombos{}, err
	}
	return DeckCombos{}, nil
}

// comboDeckList names the main board's cards the way Commander Spellbook
// expects: by printing name (both faces of a double-faced card), each card
// once, the commander and partner apart from the rest. Cards without a
// stored printing are left out.
func comboDeckList(main []deck.DeckEntry, printings map[string]Printing) (commanders []string, others []string) {
	seen := make(map[string]bool, len(main))
	add := func(list []string, e deck.DeckEntry) []string {
		p, ok := printings[e.ScryfallID]
		if !ok || p.Name == "" || seen[p.Name] {
			return list
		}
		seen[p.Name] = true
		return append(list, p.Name)
	}
	// Commanders first, so a commander's other copies don't land in others.
	for _, e := range main {
		if e.IsCommander() {
			commanders = add(commanders, e)
		}
	}
	for _, e := range main {
		if !e.IsCommander() {
			others = add(others, e)
		}
	}
	return commanders, others
}
