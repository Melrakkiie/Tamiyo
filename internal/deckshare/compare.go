package deckshare

import (
	"context"
	"errors"
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
)

type ComparedDeck struct {
	Deck      deck.Deck
	Owner     Owner
	Mine      bool
	CardCount int
}

type ComparedCard struct {
	Name           string
	ScryfallID     string
	ManaValue      float64
	Colors         *string
	CardType       *string
	ColorIdentity  *string
	Quantity       int
	OtherQuantity  int
	Commander      bool
	OtherCommander bool
}

type Comparison struct {
	Deck        ComparedDeck
	Other       ComparedDeck
	Common      []ComparedCard
	OnlyInDeck  []ComparedCard
	OnlyInOther []ComparedCard
}

func (s *Service) CompareDecks(ctx context.Context, userID string, deckID string, otherID string) (Comparison, error) {
	first, firstCards, err := s.loadVisibleDeck(ctx, userID, deckID)
	if err != nil {
		return Comparison{}, err
	}
	second, secondCards, err := s.loadVisibleDeck(ctx, userID, otherID)
	if err != nil {
		return Comparison{}, err
	}

	byName := make(map[string]*ComparedCard)
	var order []string
	add := func(cards []Card, other bool) {
		for _, c := range cards {
			key := cardNameKey(c.Name)
			entry, ok := byName[key]
			if !ok {
				entry = &ComparedCard{
					Name:          c.Name,
					ScryfallID:    c.ScryfallID,
					ManaValue:     c.ManaValue,
					Colors:        c.Colors,
					CardType:      c.CardType,
					ColorIdentity: c.ColorIdentity,
				}
				byName[key] = entry
				order = append(order, key)
			}
			if other {
				entry.OtherQuantity += c.Quantity
				entry.OtherCommander = entry.OtherCommander || c.Commander
			} else {
				entry.Quantity += c.Quantity
				entry.Commander = entry.Commander || c.Commander
			}
		}
	}
	add(firstCards, false)
	add(secondCards, true)

	sort.SliceStable(order, func(i, j int) bool {
		return strings.ToLower(byName[order[i]].Name) < strings.ToLower(byName[order[j]].Name)
	})

	comparison := Comparison{
		Deck:        first,
		Other:       second,
		Common:      []ComparedCard{},
		OnlyInDeck:  []ComparedCard{},
		OnlyInOther: []ComparedCard{},
	}
	for _, key := range order {
		c := *byName[key]
		switch {
		case c.Quantity > 0 && c.OtherQuantity > 0:
			comparison.Common = append(comparison.Common, c)
		case c.Quantity > 0:
			comparison.OnlyInDeck = append(comparison.OnlyInDeck, c)
		default:
			comparison.OnlyInOther = append(comparison.OnlyInOther, c)
		}
	}
	return comparison, nil
}

func (s *Service) loadVisibleDeck(ctx context.Context, userID string, deckID string) (ComparedDeck, []Card, error) {
	ownerID := userID
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if !errors.Is(err, deck.ErrNotFound) {
			return ComparedDeck{}, nil, err
		}
		if ownerID, d, err = s.resolve(ctx, deckID); err != nil {
			return ComparedDeck{}, nil, err
		}
	}

	owner, cards, err := s.loadContent(ctx, ownerID, d)
	if err != nil {
		return ComparedDeck{}, nil, err
	}

	total := 0
	for _, c := range cards {
		total += c.Quantity
	}
	return ComparedDeck{
		Deck:      d,
		Owner:     owner,
		Mine:      ownerID == userID,
		CardCount: total,
	}, cards, nil
}

func cardNameKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(name, "//", "/")), " "))
}
