package bulk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

type fakeScryfallFetcher struct {
	cards       []scryfall.Card
	err         error
	lastRequest []scryfall.Identifier

	setNames     map[string]string
	setNamesErr  error
	setNameCalls int
}

func (f *fakeScryfallFetcher) SetNames(ctx context.Context) (map[string]string, error) {
	f.setNameCalls++
	if f.setNamesErr != nil {
		return nil, f.setNamesErr
	}
	return f.setNames, nil
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
	expected := ResolvedCard{ScryfallID: "11111111-1111-1111-1111-111111111111", SetCode: "znr", CollectorNumber: "90", ManaValue: 3, Colors: "WG", CardType: "Creature", ColorIdentity: "WUG"}
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

func TestScryfallClient_Resolve_ResolvesByNameAndFrontFace(t *testing.T) {
	fetcher := &fakeScryfallFetcher{cards: []scryfall.Card{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "Fire // Ice", Set: "mh2", CollectorNumber: "290", CMC: 4},
	}}
	client := &ScryfallClient{client: fetcher}

	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{Name: "Fire // Ice"}})

	require.NoError(t, err)
	require.Len(t, fetcher.lastRequest, 1)
	assert.Equal(t, "Fire // Ice", fetcher.lastRequest[0].Name)
	assert.Empty(t, fetcher.lastRequest[0].Set)
	for _, name := range []string{"Fire // Ice", "fire / ice", "Fire"} {
		rc, ok := resolved[resolveKeyByName(name)]
		require.True(t, ok, name)
		assert.Equal(t, "mh2", rc.SetCode)
		assert.Equal(t, "290", rc.CollectorNumber)
		assert.Equal(t, "Fire // Ice", rc.Name)
	}
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

func TestScryfallClient_SetNamesAreCachedForADay(t *testing.T) {
	fetcher := &fakeScryfallFetcher{setNames: map[string]string{"mma": "Modern Masters"}}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	client := &ScryfallClient{client: fetcher, now: func() time.Time { return now }}

	names, err := client.SetNames(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Modern Masters", names["mma"])
	_, _ = client.SetNames(context.Background())
	assert.Equal(t, 1, fetcher.setNameCalls)

	now = now.Add(25 * time.Hour)
	fetcher.setNamesErr = errors.New("down")
	names, err = client.SetNames(context.Background())
	require.NoError(t, err, "an outdated list beats no list")
	assert.Equal(t, "Modern Masters", names["mma"])
	assert.Equal(t, 2, fetcher.setNameCalls)
}

func TestScryfallClient_SetNamesFailWithoutACachedList(t *testing.T) {
	client := &ScryfallClient{client: &fakeScryfallFetcher{setNamesErr: errors.New("down")}, now: time.Now}

	_, err := client.SetNames(context.Background())

	assert.Error(t, err)
}
