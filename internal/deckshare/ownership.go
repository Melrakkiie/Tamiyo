package deckshare

import (
	"context"
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
)

type OwnedCard struct {
	Name  string
	Owned int
}

func (s *Service) CollectionOwnership(ctx context.Context, viewerID string, deckID string) ([]OwnedCard, error) {
	read, err := s.decks.ReadDeck(ctx, viewerID, deckID, deck.ReachVisible)
	if err != nil {
		return nil, err
	}

	names := make(map[string]string)
	for _, e := range read.Entries {
		key := deck.CardNameKey(e.Name)
		if _, seen := names[key]; !seen {
			names[key] = e.Name
		}
	}
	keys := make([]string, 0, len(names))
	for key := range names {
		keys = append(keys, key)
	}

	counts, err := s.decks.CountCopiesByName(ctx, viewerID, keys)
	if err != nil {
		return nil, err
	}

	owned := make([]OwnedCard, 0, len(names))
	for key, name := range names {
		owned = append(owned, OwnedCard{Name: name, Owned: counts[key]})
	}
	sort.Slice(owned, func(i, j int) bool { return strings.ToLower(owned[i].Name) < strings.ToLower(owned[j].Name) })
	return owned, nil
}
