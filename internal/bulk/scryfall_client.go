package bulk

import (
	"context"
	"sync"
	"time"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

const setNamesTTL = 24 * time.Hour

type scryfallFetcher interface {
	Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error)
	SetNames(ctx context.Context) (map[string]string, error)
}

type ScryfallClient struct {
	client scryfallFetcher
	now    func() time.Time

	mu            sync.Mutex
	setNames      map[string]string
	setNamesUntil time.Time
}

func NewScryfallClient() *ScryfallClient {
	return &ScryfallClient{client: scryfall.NewClient(), now: time.Now}
}

func (c *ScryfallClient) SetNames(ctx context.Context) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setNames != nil && c.now().Before(c.setNamesUntil) {
		return c.setNames, nil
	}
	names, err := c.client.SetNames(ctx)
	if err != nil {
		if c.setNames != nil {
			return c.setNames, nil
		}
		return nil, err
	}
	c.setNames = names
	c.setNamesUntil = c.now().Add(setNamesTTL)
	return names, nil
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
