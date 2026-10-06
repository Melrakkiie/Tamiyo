package bulk

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

type fakeScryfallFetcher struct {
	cards       []scryfall.Card
	err         error
	lastRequest []scryfall.Identifier
}

func (f *fakeScryfallFetcher) Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error) {
	f.lastRequest = identifiers
	if f.err != nil {
		return nil, f.err
	}
	return f.cards, nil
}

func TestScryfallClient_Resolve_TranslatesIdentifiersAndBuildsMap(t *testing.T) {
	fetcher := &fakeScryfallFetcher{cards: []scryfall.Card{
		{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90", CMC: 3, TypeLine: "Creature — Elf Druid", Colors: []string{"G", "W"}, ColorIdentity: []string{"G", "W", "U"}},
	}}
	client := &ScryfallClient{client: fetcher}

	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "ZNR", CollectorNumber: "90"}})

	require.NoError(t, err)
	expected := ResolvedCard{ScryfallID: "11111111-1111-1111-1111-111111111111", ManaValue: 3, Colors: "WG", CardType: "Creature", ColorIdentity: "WUG"}
	assert.Equal(t, expected, resolved[resolveKey("ZNR", "90")])
	assert.Equal(t, expected, resolved[resolveKeyByID("11111111-1111-1111-1111-111111111111")])
	require.Len(t, fetcher.lastRequest, 1)
	assert.Equal(t, "ZNR", fetcher.lastRequest[0].Set)
	assert.Equal(t, "90", fetcher.lastRequest[0].CollectorNumber)
}

func TestScryfallClient_Resolve_ResolvesByScryfallID(t *testing.T) {
	fetcher := &fakeScryfallFetcher{cards: []scryfall.Card{
		{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90", CMC: 3},
	}}
	client := &ScryfallClient{client: fetcher}

	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{ScryfallID: "11111111-1111-1111-1111-111111111111"}})

	require.NoError(t, err)
	assert.Equal(t, float64(3), resolved[resolveKeyByID("11111111-1111-1111-1111-111111111111")].ManaValue)
	require.Len(t, fetcher.lastRequest, 1)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", fetcher.lastRequest[0].ID)
}

func TestScryfallClient_Resolve_OmitsCardsNotReturned(t *testing.T) {
	fetcher := &fakeScryfallFetcher{cards: nil}
	client := &ScryfallClient{client: fetcher}

	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "xxx", CollectorNumber: "999"}})

	require.NoError(t, err)
	assert.Empty(t, resolved)
}

func TestScryfallClient_Resolve_PropagatesFetchError(t *testing.T) {
	fetcher := &fakeScryfallFetcher{err: errors.New("network down")}
	client := &ScryfallClient{client: fetcher}

	_, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "znr", CollectorNumber: "90"}})

	require.Error(t, err)
}

func TestNewScryfallClient_ReturnsUsableClient(t *testing.T) {
	client := NewScryfallClient()

	require.NotNil(t, client)
	require.NotNil(t, client.client)
}
