package printing

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

type fakeRepo struct {
	batches         [][]string
	upserted        []Printing
	markedRefreshed []string
	lastStale       time.Time
	lastLimit       int
	err             error
}

func (f *fakeRepo) IDsToRefresh(ctx context.Context, staleBefore time.Time, limit int) ([]string, error) {
	f.lastStale = staleBefore
	f.lastLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	if len(f.batches) == 0 {
		return nil, nil
	}
	next := f.batches[0]
	f.batches = f.batches[1:]
	return next, nil
}

func (f *fakeRepo) Upsert(ctx context.Context, printings []Printing) error {
	f.upserted = append(f.upserted, printings...)
	return nil
}

func (f *fakeRepo) MarkRefreshed(ctx context.Context, ids []string) error {
	f.markedRefreshed = append(f.markedRefreshed, ids...)
	return nil
}

type fakeFetcher struct {
	cards     []scryfall.Card
	rejected  []string
	err       error
	requested []scryfall.Identifier
}

func (f *fakeFetcher) Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error) {
	f.requested = append(f.requested, identifiers...)
	var rejected []scryfall.Identifier
	for _, id := range identifiers {
		if slices.Contains(f.rejected, id.ID) {
			rejected = append(rejected, id)
		}
	}
	if rejected != nil {
		return f.cards, &scryfall.RejectedError{Identifiers: rejected}
	}
	return f.cards, f.err
}

func TestFromScryfall_KeepsLegalAndRestrictedFormatsSorted(t *testing.T) {
	p := FromScryfall(scryfall.Card{
		ID:         "a",
		TypeLine:   "Artifact",
		Legalities: map[string]string{"vintage": "restricted", "commander": "legal", "modern": "banned", "standard": "not_legal"},
	})

	assert.Equal(t, []string{"commander", "vintage"}, p.LegalFormats)
}

func TestFromScryfall_SplitsBannedAndRestrictedFormatsSorted(t *testing.T) {
	p := FromScryfall(scryfall.Card{
		ID: "a",
		Legalities: map[string]string{
			"vintage": "restricted", "commander": "legal", "modern": "banned", "legacy": "banned",
			"standard": "not_legal", "pauper": "restricted",
		},
	})

	assert.Equal(t, []string{"legacy", "modern"}, p.BannedFormats)
	assert.Equal(t, []string{"pauper", "vintage"}, p.RestrictedFormats)
}

func TestFromScryfall_KeepsColorsAndColorIdentity(t *testing.T) {
	p := FromScryfall(scryfall.Card{ID: "a", Colors: []string{"G", "U"}, ColorIdentity: []string{"G", "U", "W"}})

	assert.Equal(t, []string{"G", "U"}, p.Colors)
	assert.Equal(t, []string{"G", "U", "W"}, p.ColorIdentity)
}

func TestFromScryfall_StoresEmptyListsForAColorlessUnrestrictedCard(t *testing.T) {
	p := FromScryfall(scryfall.Card{ID: "a", TypeLine: "Artifact", Legalities: map[string]string{"commander": "legal"}})

	assert.Equal(t, []string{}, p.Colors)
	assert.Equal(t, []string{}, p.ColorIdentity)
	assert.Equal(t, []string{}, p.BannedFormats)
	assert.Equal(t, []string{}, p.RestrictedFormats)
}

func TestFromScryfall_KeepsTheManaValue(t *testing.T) {
	p := FromScryfall(scryfall.Card{ID: "a", CMC: 3})

	assert.Equal(t, 3.0, p.ManaValue)
}

func TestFromScryfall_KeepsTheGameChangerFlag(t *testing.T) {
	assert.True(t, FromScryfall(scryfall.Card{ID: "a", GameChanger: true}).GameChanger)
	assert.False(t, FromScryfall(scryfall.Card{ID: "a"}).GameChanger)
}

func TestFromScryfall_KeepsTheEnglishName(t *testing.T) {
	p := FromScryfall(scryfall.Card{ID: "a", Name: "Sol Ring"})

	assert.Equal(t, "Sol Ring", p.Name)
}

func TestFromScryfall_KeepsPrices(t *testing.T) {
	eur, usdFoil := 2.5, 9.99
	prices := scryfall.Prices{EUR: &eur, USDFoil: &usdFoil}

	p := FromScryfall(scryfall.Card{ID: "a", Prices: prices})

	assert.Equal(t, prices, p.Prices)
}

func TestFromScryfall_KeepsPowersAndToughnesses(t *testing.T) {
	p := FromScryfall(scryfall.Card{ID: "a", Powers: []string{"1", "3"}, Toughnesses: []string{"1", "2"}})

	assert.Equal(t, []string{"1", "3"}, p.Powers)
	assert.Equal(t, []string{"1", "2"}, p.Toughnesses)
}

func TestFromScryfall_KeepsSearchFields(t *testing.T) {
	p := FromScryfall(scryfall.Card{ID: "a", ManaCost: "{G}", OracleText: "Trample", Rarity: "rare", Keywords: []string{"Trample"}})

	assert.Equal(t, "{G}", p.ManaCost)
	assert.Equal(t, "Trample", p.OracleText)
	assert.Equal(t, "rare", p.Rarity)
	assert.Equal(t, []string{"Trample"}, p.Keywords)
}

func TestRefreshBatch_StoresFoundAndMissingPrintings(t *testing.T) {
	repo := &fakeRepo{batches: [][]string{{"AAAA", "bbbb"}}}
	fetcher := &fakeFetcher{cards: []scryfall.Card{{ID: "aaaa", TypeLine: "Creature — Elf", Legalities: map[string]string{"commander": "legal"}}}}
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	refresher := NewRefresher(repo, fetcher, zap.NewNop())
	refresher.now = func() time.Time { return now }

	count, err := refresher.RefreshBatch(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, now.Add(-24*time.Hour), repo.lastStale)
	assert.Equal(t, refreshBatchSize, repo.lastLimit)
	assert.Equal(t, []scryfall.Identifier{{ID: "AAAA"}, {ID: "bbbb"}}, fetcher.requested)
	assert.Equal(t, []Printing{
		{
			ScryfallID: "AAAA", TypeLine: "Creature — Elf", LegalFormats: []string{"commander"},
			BannedFormats: []string{}, RestrictedFormats: []string{}, Colors: []string{}, ColorIdentity: []string{},
		},
		{
			ScryfallID: "bbbb", LegalFormats: []string{}, BannedFormats: []string{}, RestrictedFormats: []string{},
			Colors: []string{}, ColorIdentity: []string{}, Powers: []string{}, Toughnesses: []string{}, Keywords: []string{},
		},
	}, repo.upserted)
}

func TestRefreshBatch_NothingToDo(t *testing.T) {
	fetcher := &fakeFetcher{}
	count, err := NewRefresher(&fakeRepo{}, fetcher, zap.NewNop()).RefreshBatch(context.Background())

	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Empty(t, fetcher.requested)
}

func TestRefreshBatch_ScryfallErrorStoresNothing(t *testing.T) {
	repo := &fakeRepo{batches: [][]string{{"a"}}}
	_, err := NewRefresher(repo, &fakeFetcher{err: errors.New("down")}, zap.NewNop()).RefreshBatch(context.Background())

	require.Error(t, err)
	assert.Empty(t, repo.upserted)
}

func TestRefreshAll_StopsWhenEverythingIsFresh(t *testing.T) {
	repo := &fakeRepo{batches: [][]string{{"a"}, {"b"}}}
	fetcher := &fakeFetcher{}

	NewRefresher(repo, fetcher, zap.NewNop()).refreshAll(context.Background())

	assert.Len(t, repo.upserted, 2)
}

func TestRefreshAll_KeepsARejectedPrintingAndRefreshesTheRestOfTheRun(t *testing.T) {
	repo := &fakeRepo{batches: [][]string{{"a", "rejected"}, {"c"}}}
	fetcher := &fakeFetcher{
		cards:    []scryfall.Card{{ID: "a", Name: "Sol Ring"}, {ID: "c", Name: "Brainstorm"}},
		rejected: []string{"rejected"},
	}

	NewRefresher(repo, fetcher, zap.NewNop()).refreshAll(context.Background())

	names := map[string]string{}
	for _, p := range repo.upserted {
		names[p.ScryfallID] = p.Name
	}
	assert.Equal(t, map[string]string{"a": "Sol Ring", "c": "Brainstorm"}, names)
	assert.Equal(t, []string{"rejected"}, repo.markedRefreshed)
}

func TestRun_StopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewRefresher(&fakeRepo{}, &fakeFetcher{}, zap.NewNop()).Run(ctx)
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}
