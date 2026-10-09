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
	ownerID, d, err := s.resolveVisible(ctx, viewerID, deckID)
	if err != nil {
		return nil, err
	}
	_, cards, err := s.loadContent(ctx, ownerID, d)
	if err != nil {
		return nil, err
	}

	names := make(map[string]string)
	for _, c := range cards {
		key := deck.CardNameKey(c.Name)
		if _, seen := names[key]; !seen {
			names[key] = c.Name
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
