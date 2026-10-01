package bulk

import (
	"context"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

type scryfallFetcher interface {
	Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error)
}

type ScryfallClient struct {
	client scryfallFetcher
}

func NewScryfallClient() *ScryfallClient {
	return &ScryfallClient{client: scryfall.NewClient()}
}

func (c *ScryfallClient) Resolve(ctx context.Context, identifiers []CardIdentifier) (map[string]string, error) {
	wire := make([]scryfall.Identifier, len(identifiers))
	for i, id := range identifiers {
		wire[i] = scryfall.Identifier{Set: id.SetCode, CollectorNumber: id.CollectorNumber}
	}

	cards, err := c.client.Fetch(ctx, wire)
	if err != nil {
		return nil, err
	}

	resolved := make(map[string]string, len(cards))
	for _, card := range cards {
		resolved[resolveKey(card.Set, card.CollectorNumber)] = card.ID
	}
	return resolved, nil
}
