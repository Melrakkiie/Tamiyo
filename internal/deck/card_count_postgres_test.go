//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepository_EveryRouteReturnsTheSameCardCount(t *testing.T) {
	f := newContentFixture(t)
	ctx := context.Background()

	// Main board: 3 owned bolts (one proxy), an owned commander, a pending partner and 2 pending islands.
	f.ownedCopy(t, bolt, copyOptions{})
	f.ownedCopy(t, bolt, copyOptions{})
	f.ownedCopy(t, bolt, copyOptions{proxy: true})
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	partner := f.pendingCard(t, tymna, 1, copyOptions{})
	f.pendingCard(t, island, 2, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)
	f.ownedCopy(t, bolt, copyOptions{board: BoardSideboard})
	f.pendingCard(t, island, 3, copyOptions{board: BoardSideboard})
	f.ownedCopy(t, bolt, copyOptions{board: BoardConsidering})
	f.pendingCard(t, island, 4, copyOptions{board: BoardConsidering})
	f.setVisibility(t, VisibilityPublic)

	const wantCount, wantPending = 7, 3

	goCount := 0
	for _, e := range f.content(t).Entries {
		if InMainBoard(e.Board) {
			goCount += e.Quantity()
		}
	}
	assert.Equal(t, wantCount, goCount)

	one, err := f.repo.FindByID(ctx, f.userID, contentDeckID)
	require.NoError(t, err)
	assert.Equal(t, wantCount, one.CardCount)
	assert.Equal(t, wantPending, one.PendingCount)

	list, _, err := f.repo.FindAll(ctx, f.userID, Filter{Page: 1, Limit: 24, SortField: "name"})
	require.NoError(t, err)
	listed := findDeck(t, list, func(d Deck) string { return d.ID })
	assert.Equal(t, wantCount, listed.CardCount)
	assert.Equal(t, wantPending, listed.PendingCount)

	_, shared, err := f.repo.FindShared(ctx, contentDeckID)
	require.NoError(t, err)
	assert.Equal(t, wantCount, shared.CardCount)
	assert.Equal(t, wantPending, shared.PendingCount)

	public, _, err := f.repo.FindPublic(ctx, PublicFilter{Page: 1, Limit: 24, SortField: "card_count", SortDesc: true})
	require.NoError(t, err)
	browsed := findDeck(t, public, func(d PublicDeck) string { return d.ID })
	assert.Equal(t, wantCount, browsed.CardCount)
}

func findDeck[T any](t *testing.T, decks []T, id func(T) string) T {
	t.Helper()
	for _, d := range decks {
		if id(d) == contentDeckID {
			return d
		}
	}
	require.FailNow(t, "deck not in the results")
	var none T
	return none
}
