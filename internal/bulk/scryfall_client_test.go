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
		{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90"},
	}}
	client := &ScryfallClient{client: fetcher}

	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "ZNR", CollectorNumber: "90"}})

	require.NoError(t, err)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", resolved[resolveKey("ZNR", "90")])
	require.Len(t, fetcher.lastRequest, 1)
	assert.Equal(t, "ZNR", fetcher.lastRequest[0].Set)
	assert.Equal(t, "90", fetcher.lastRequest[0].CollectorNumber)
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
