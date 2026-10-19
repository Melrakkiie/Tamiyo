//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f contentFixture) duplicate(t *testing.T, userID string, req DuplicateRequest) DuplicateResult {
	t.Helper()
	result, err := f.service.DuplicateDeck(context.Background(), userID, contentDeckID, req)
	require.NoError(t, err)
	return result
}

func (f contentFixture) contentOf(t *testing.T, userID string, deckID string) DeckContent {
	t.Helper()
	content, err := f.service.GetDeckContent(context.Background(), userID, deckID)
	require.NoError(t, err)
	return content
}

func (f contentFixture) deckCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.Get(&n, `SELECT count(*) FROM tamiyo.deck`))
	return n
}

func TestDuplicateDeck_CopiesBoardsFinishesProxiesAndCommanders(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	partner := f.pendingCard(t, tymna, 1, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)
	f.ownedCopy(t, bolt, copyOptions{foil: true})
	f.pendingCard(t, bolt, 2, copyOptions{proxy: true})
	f.pendingCard(t, island, 1, copyOptions{board: BoardSideboard})
	f.pendingCard(t, island, 1, copyOptions{board: BoardConsidering})
	source := f.content(t)

	result := f.duplicate(t, f.userID, DuplicateRequest{})

	copied := f.contentOf(t, f.userID, result.Deck.ID)
	assert.NotEqual(t, contentDeckID, result.Deck.ID)
	assert.Equal(t, "Otterly Playful (copie)", copied.Deck.Name)
	assert.Equal(t, "modern", copied.Deck.Format)
	assert.Equal(t, VisibilityPrivate, copied.Deck.Visibility)
	assert.Equal(t, summarize(source.Entries), summarize(copied.Entries))
	assert.Equal(t, 7, result.Placement.CardsLinked+result.Placement.CardsPending)
	assert.Equal(t, source, f.content(t), "the source deck is untouched")
}

func TestDuplicateDeck_UsesTheUsersFreeOwnedCopiesFirst(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 2, copyOptions{})
	free := f.collectionCopy(t, collectionCopy{printing: bolt})

	result := f.duplicate(t, f.userID, DuplicateRequest{Name: "Burn"})

	assert.Equal(t, PlacementSummary{CardsLinked: 1, CardsPending: 1}, result.Placement)
	copied := f.contentOf(t, f.userID, result.Deck.ID)
	assert.Equal(t, "Burn", copied.Deck.Name)
	require.Len(t, copied.Entries, 1)
	assert.Equal(t, []int{free}, copied.Entries[0].OwnedCardIDs)
	assert.Equal(t, 1, copied.Entries[0].PendingQuantity())
}

func TestDuplicateDeck_CopiesASharedDeckOfAnotherUserWithoutScryfall(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, nil)
	f.ownedCopy(t, bolt, copyOptions{foil: true})
	_, err := f.db.Exec(`UPDATE tamiyo.deck SET visibility = 'unlisted' WHERE id = $1`, contentDeckID)
	require.NoError(t, err)
	bob := contentFixture{db: f.db, repo: f.repo, service: f.service, userID: seedUser(t, f.db, "bob@example.com")}
	bobsCopy := bob.collectionCopy(t, collectionCopy{printing: bolt, foil: true})

	result := f.duplicate(t, bob.userID, DuplicateRequest{Visibility: VisibilityPublic})

	copied := f.contentOf(t, bob.userID, result.Deck.ID)
	assert.Equal(t, VisibilityPublic, copied.Deck.Visibility)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, pending: 1, quantity: 1},
		{name: bolt.name, board: BoardMain, foil: true, owned: 1, quantity: 1},
	}, summarize(copied.Entries))
	assert.Equal(t, []int{bobsCopy}, copied.Entries[1].OwnedCardIDs, "only bob's own copies are used")
}

func TestDuplicateDeck_RefusesAPrivateDeckOfAnotherUser(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{})
	_, err := f.db.Exec(`UPDATE tamiyo.deck SET visibility = 'private' WHERE id = $1`, contentDeckID)
	require.NoError(t, err)
	bob := seedUser(t, f.db, "bob@example.com")
	decks := f.deckCount(t)

	_, err = f.service.DuplicateDeck(context.Background(), bob, contentDeckID, DuplicateRequest{})

	assert.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, decks, f.deckCount(t))
}

func TestDuplicateDeck_CopiesTheTagsOfTheCards(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{})
	f.pendingCard(t, island, 1, copyOptions{})
	_, err := f.service.SetCardTags(context.Background(), f.userID, contentDeckID, bolt.name, []string{"Burn", "Cheap"})
	require.NoError(t, err)

	result := f.duplicate(t, f.userID, DuplicateRequest{})

	tags, err := f.service.GetCardTags(context.Background(), f.userID, result.Deck.ID)
	require.NoError(t, err)
	assert.Equal(t, []TaggedCard{{Name: bolt.name, Tags: []string{"Burn", "Cheap"}}}, tags.Cards)
}

func TestDuplicateDeck_ReportsAnUnknownFolderAndCreatesNothing(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})
	folder := 999999
	decks := f.deckCount(t)

	_, err := f.service.DuplicateDeck(context.Background(), f.userID, contentDeckID, DuplicateRequest{FolderID: &folder})

	assert.ErrorIs(t, err, ErrTargetFolderNotFound)
	assert.Equal(t, decks, f.deckCount(t))
}

func TestDuplicateDeck_LeavesNoDeckBehindWhenPlacingFails(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})
	f.pendingCard(t, island, 1, copyOptions{})
	failNthWrite(t, f.db, "tamiyo.deck_pending_cards", "INSERT", 2)
	decks := f.deckCount(t)

	f.assertRolledBack(t, func() error {
		_, err := f.service.DuplicateDeck(context.Background(), f.userID, contentDeckID, DuplicateRequest{})
		return err
	})

	assert.Equal(t, decks, f.deckCount(t))
}
