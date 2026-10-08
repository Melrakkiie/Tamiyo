package printing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

type fakeRepo struct {
	batches   [][]string
	upserted  []Printing
	lastStale time.Time
	lastLimit int
	err       error
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

type fakeFetcher struct {
	cards     []scryfall.Card
	err       error
	requested []scryfall.Identifier
}

func (f *fakeFetcher) Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error) {
	f.requested = append(f.requested, identifiers...)
	return f.cards, f.err
}

func TestFromScryfall_KeepsLegalAndRestrictedFormatsSorted(t *testing.T) {
	p := FromScryfall(scryfall.Card{
		ID:         "a",
		TypeLine:   "Artifact",
		Legalities: map[string]string{"vintage": "restricted", "commander": "legal", "modern": "banned", "standard": "not_legal"},
	})

	assert.Equal(t, Printing{ScryfallID: "a", TypeLine: "Artifact", LegalFormats: []string{"commander", "vintage"}}, p)
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
		{ScryfallID: "AAAA", TypeLine: "Creature — Elf", LegalFormats: []string{"commander"}},
		{ScryfallID: "bbbb", LegalFormats: []string{}},
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
