package deckinsights

import (
	"context"
	"errors"
	"fmt"

	"Melrakkiie/Tamiyo/internal/commanderspellbook"
	"Melrakkiie/Tamiyo/internal/deck"
)

// ErrCombosUnavailable is returned when Commander Spellbook can't be reached
// or doesn't answer usably.
var ErrCombosUnavailable = errors.New("commander spellbook is unavailable")

// comboFinder looks a deck's combos up on Commander Spellbook (ADR 0011).
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

type DeckCombos struct {
	Combos []Combo `json:"combos"`
}

func (s *Service) GetDeckCombos(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (DeckCombos, error) {
	_, main, printings, err := s.loadMainBoard(ctx, viewerID, deckID, reach)
	if err != nil {
		return DeckCombos{}, err
	}

	commanders, others := comboDeckList(main, printings)
	if len(commanders) == 0 && len(others) == 0 {
		return DeckCombos{Combos: []Combo{}}, nil
	}

	found, err := s.combos.FindCombos(ctx, commanders, others)
	if err != nil {
		return DeckCombos{}, fmt.Errorf("%w: %w", ErrCombosUnavailable, err)
	}
	combos := make([]Combo, 0, len(found))
	for _, c := range found {
		combos = append(combos, Combo{
			ID:      c.ID,
			Cards:   c.Cards,
			Results: c.Results,
			URL:     c.URL(),
		})
	}
	return DeckCombos{Combos: combos}, nil
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
