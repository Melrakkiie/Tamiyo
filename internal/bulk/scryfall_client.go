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

func (c *ScryfallClient) Resolve(ctx context.Context, identifiers []CardIdentifier) (map[string]ResolvedCard, error) {
	wire := make([]scryfall.Identifier, len(identifiers))
	for i, id := range identifiers {
		if id.ScryfallID != "" {
			wire[i] = scryfall.Identifier{ID: id.ScryfallID}
		} else if id.SetCode == "" && id.Name != "" {
			wire[i] = scryfall.Identifier{Name: id.Name}
		} else {
			wire[i] = scryfall.Identifier{Set: id.SetCode, CollectorNumber: id.CollectorNumber}
		}
	}

	cards, err := c.client.Fetch(ctx, wire)
	if err != nil {
		return nil, err
	}

	resolved := make(map[string]ResolvedCard, len(cards)*2)
	for _, card := range cards {
		rc := ResolvedCard{
			ScryfallID:      card.ID,
			Name:            card.Name,
			SetCode:         card.Set,
			CollectorNumber: card.CollectorNumber,
			ManaValue:       card.CMC,
			Colors:          scryfall.ColorCode(card.Colors),
			CardType:        scryfall.PrimaryType(card.TypeLine),
			ColorIdentity:   scryfall.ColorCode(card.ColorIdentity),
		}
		resolved[resolveKey(card.Set, card.CollectorNumber)] = rc
		resolved[resolveKeyByID(card.ID)] = rc
		if card.Name != "" {
			resolved[resolveKeyByName(card.Name)] = rc
			resolved[resolveKeyByName(frontFaceName(card.Name))] = rc
		}
	}
	return resolved, nil
}
