//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f contentFixture) collectOwn(t *testing.T, req OwnDeckCollect) CollectSummary {
	t.Helper()
	summary, err := f.service.CollectOwnDeck(context.Background(), f.userID, contentDeckID, req)
	require.NoError(t, err)
	return summary
}

func TestCollectOwnDeck_PendingCommitsThePendingCardsOfTheChosenBoards(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, island, copyOptions{})
	f.pendingCard(t, bolt, 2, copyOptions{})
	f.pendingCard(t, tymna, 1, copyOptions{board: BoardSideboard, foil: true})
	f.pendingCard(t, atraxa, 1, copyOptions{board: BoardConsidering})
	before := f.collection(t)

	summary := f.collectOwn(t, OwnDeckCollect{Boards: []string{BoardMain, BoardSideboard}})

	assert.Equal(t, CollectSummary{CardsCreated: 3, CardsLinked: 3}, summary)
	assert.Equal(t, []entrySummary{
		{name: island.name, board: BoardMain, owned: 1, quantity: 1},
		{name: bolt.name, board: BoardMain, owned: 2, quantity: 2},
		{name: tymna.name, board: BoardSideboard, foil: true, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardConsidering, pending: 1, quantity: 1},
	}, summarize(f.content(t).Entries))
	assert.Equal(t, append(before,
		collectionCard{name: bolt.name, scryfallID: bolt.scryfallID},
		collectionCard{name: bolt.name, scryfallID: bolt.scryfallID},
		collectionCard{name: tymna.name, scryfallID: tymna.scryfallID, foil: true},
	), f.collection(t))
}

func TestCollectOwnDeck_AllAlsoCreatesACopyOfEachOwnedCardOutOfTheDeck(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, island, copyOptions{foil: true})
	f.ownedCopy(t, bolt, copyOptions{proxy: true})
	f.ownedCopy(t, tymna, copyOptions{board: BoardConsidering})
	f.pendingCard(t, bolt, 1, copyOptions{proxy: true})
	box := f.storage(t, "Box", false)

	summary := f.collectOwn(t, OwnDeckCollect{Boards: []string{BoardMain}, StorageID: box, OwnedCopies: true})

	assert.Equal(t, CollectSummary{CardsCreated: 3, CardsLinked: 1}, summary)
	assert.Equal(t, []entrySummary{
		{name: island.name, board: BoardMain, foil: true, owned: 1, quantity: 1},
		{name: bolt.name, board: BoardMain, proxy: true, owned: 2, quantity: 2},
		{name: tymna.name, board: BoardConsidering, owned: 1, quantity: 1},
	}, summarize(f.content(t).Entries))
	collection := f.collection(t)
	require.Len(t, collection, 6)
	assert.ElementsMatch(t, []collectionCard{
		{name: bolt.name, scryfallID: bolt.scryfallID, proxy: true, storageID: box},
		{name: island.name, scryfallID: island.scryfallID, foil: true, storageID: box},
		{name: bolt.name, scryfallID: bolt.scryfallID, proxy: true, storageID: box},
	}, collection[3:])
}

func TestCollectOwnDeck_MovesAPendingCommanderToItsFirstCopy(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 2, copyOptions{})
	partner := f.pendingCard(t, tymna, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, &partner)

	summary := f.collectOwn(t, OwnDeckCollect{Boards: []string{BoardMain}})

	assert.Equal(t, CollectSummary{CardsCreated: 3, CardsLinked: 3}, summary)
	content := f.content(t)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, slot: SlotPartner, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, owned: 1, quantity: 1},
	}, summarize(content.Entries))
	assert.Nil(t, content.Deck.CommanderPendingID)
	assert.Nil(t, content.Deck.PartnerPendingID)
}

func TestCollectOwnDeck_LeavesEverythingAsBeforeWhenACopyFailsAfterTheCommits(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, nil)
	f.pendingCard(t, bolt, 2, copyOptions{})
	f.ownedCopy(t, island, copyOptions{})
	f.ownedCopy(t, island, copyOptions{})
	failNthWrite(t, f.db, "tamiyo.cards", "INSERT", 4)

	f.assertRolledBack(t, func() error {
		_, err := f.service.CollectOwnDeck(context.Background(), f.userID, contentDeckID, OwnDeckCollect{Boards: []string{BoardMain}, OwnedCopies: true})
		return err
	})
}

func TestCollectOwnDeck_RetryingAfterAFailureCreatesNoExtraCopy(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, island, copyOptions{})
	f.pendingCard(t, bolt, 2, copyOptions{})
	failNthWrite(t, f.db, "tamiyo.deck_pending_cards", "DELETE", 1)
	_, err := f.service.CollectOwnDeck(context.Background(), f.userID, contentDeckID, OwnDeckCollect{Boards: []string{BoardMain}, OwnedCopies: true})
	require.Error(t, err)

	summary := f.collectOwn(t, OwnDeckCollect{Boards: []string{BoardMain}, OwnedCopies: true})

	assert.Equal(t, CollectSummary{CardsCreated: 3, CardsLinked: 2}, summary)
	assert.Len(t, f.collection(t), 4)
}

func TestCollectOwnDeck_RejectsAnUnknownStorage(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})
	f.ownedCopy(t, island, copyOptions{})

	f.assertRolledBack(t, func() error {
		_, err := f.service.CollectOwnDeck(context.Background(), f.userID, contentDeckID, OwnDeckCollect{Boards: []string{BoardMain}, StorageID: intPtr(999), OwnedCopies: true})
		assert.ErrorIs(t, err, ErrTargetStorageNotFound)
		return err
	})
}

func TestCollectOwnDeck_IsIsolatedFromAnotherUser(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})
	f.ownedCopy(t, island, copyOptions{})
	bob := seedUser(t, f.db, "bob@example.com")

	f.assertRolledBack(t, func() error {
		_, err := f.service.CollectOwnDeck(context.Background(), bob, contentDeckID, OwnDeckCollect{Boards: []string{BoardMain}, OwnedCopies: true})
		assert.ErrorIs(t, err, ErrNotFound)
		return err
	})
	assert.Empty(t, f.collectionOf(t, bob))
}

// sharedWithBob makes the fixture's deck public and returns another user,
// bob, who can see it.
func (f contentFixture) sharedWithBob(t *testing.T) string {
	t.Helper()
	f.setVisibility(t, VisibilityPublic)
	return seedUser(t, f.db, "bob@example.com")
}

func (f contentFixture) setVisibility(t *testing.T, visibility string) {
	t.Helper()
	_, err := f.db.Exec(`UPDATE tamiyo.deck SET visibility = $2 WHERE id = $1`, contentDeckID, visibility)
	require.NoError(t, err)
}

func (f contentFixture) looseCard(t *testing.T, userID string, p printing) {
	t.Helper()
	_, err := f.db.Exec(`
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, mana_value) VALUES ($1, $2, $3, $4, $5, false, 1)
	`, userID, p.name, p.scryfallID, p.setCode, p.collectorNumber)
	require.NoError(t, err)
}

func (f contentFixture) collectShared(t *testing.T, userID string, req SharedDeckCollect) CollectSummary {
	t.Helper()
	summary, err := f.service.CollectSharedDeck(context.Background(), userID, contentDeckID, req)
	require.NoError(t, err)
	return summary
}

func TestCollectSharedDeck_AllCreatesEveryCopyOfTheChosenBoardsInTheViewersCollection(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{foil: true})
	f.pendingCard(t, bolt, 2, copyOptions{foil: true})
	f.pendingCard(t, island, 1, copyOptions{board: BoardSideboard, proxy: true})
	f.ownedCopy(t, atraxa, copyOptions{board: BoardConsidering})
	bob := f.sharedWithBob(t)
	before := f.state(t)

	summary := f.collectShared(t, bob, SharedDeckCollect{Boards: []string{BoardMain, BoardSideboard}})

	assert.Equal(t, CollectSummary{CardsCreated: 4}, summary)
	assert.Equal(t, []collectionCard{
		{name: bolt.name, scryfallID: bolt.scryfallID, foil: true},
		{name: bolt.name, scryfallID: bolt.scryfallID, foil: true},
		{name: bolt.name, scryfallID: bolt.scryfallID, foil: true},
		{name: island.name, scryfallID: island.scryfallID, proxy: true},
	}, f.collectionOf(t, bob))
	assert.Equal(t, before, f.state(t), "the shared deck and its owner's collection are untouched")
}

func TestCollectSharedDeck_MissingSkipsCopiesAlreadyOwnedInAnyPrinting(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{foil: true})
	f.pendingCard(t, bolt, 2, copyOptions{})
	f.pendingCard(t, island, 2, copyOptions{board: BoardSideboard})
	f.pendingCard(t, tymna, 1, copyOptions{})
	bob := f.sharedWithBob(t)
	otherBolt := printing{bolt.name, "11111111-1111-4111-8111-111111111111", "m10", "146"}
	f.looseCard(t, bob, otherBolt)
	f.looseCard(t, bob, island)
	f.looseCard(t, bob, island)
	f.looseCard(t, bob, island)

	summary := f.collectShared(t, bob, SharedDeckCollect{Boards: []string{BoardMain, BoardSideboard}, MissingOnly: true})

	assert.Equal(t, CollectSummary{CardsCreated: 3}, summary)
	assert.Equal(t, []collectionCard{
		{name: bolt.name, scryfallID: bolt.scryfallID},
		{name: bolt.name, scryfallID: bolt.scryfallID, foil: true},
		{name: tymna.name, scryfallID: tymna.scryfallID},
	}, f.collectionOf(t, bob)[4:])
}

func TestCollectSharedDeck_LeavesTheViewersCollectionAsBeforeWhenACopyFails(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 2, copyOptions{})
	f.pendingCard(t, island, 2, copyOptions{})
	bob := f.sharedWithBob(t)
	f.looseCard(t, bob, tymna)
	before := f.collectionOf(t, bob)
	failNthWrite(t, f.db, "tamiyo.cards", "INSERT", 3)

	_, err := f.service.CollectSharedDeck(context.Background(), bob, contentDeckID, SharedDeckCollect{Boards: []string{BoardMain}})

	require.Error(t, err)
	assert.Equal(t, before, f.collectionOf(t, bob))
}

func TestCollectSharedDeck_IsIsolatedFromAPrivateDeckAndAnotherUsersStorage(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})
	bob := seedUser(t, f.db, "bob@example.com")
	f.setVisibility(t, VisibilityPrivate)

	_, err := f.service.CollectSharedDeck(context.Background(), bob, contentDeckID, SharedDeckCollect{Boards: []string{BoardMain}})
	assert.ErrorIs(t, err, ErrNotFound)

	f.setVisibility(t, VisibilityUnlisted)
	alicesBox := f.storage(t, "Box", false)
	_, err = f.service.CollectSharedDeck(context.Background(), bob, contentDeckID, SharedDeckCollect{Boards: []string{BoardMain}, StorageID: alicesBox})
	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
	assert.Empty(t, f.collectionOf(t, bob))
}

func TestCollectOwnDeck_LeavesTheUpdatedDateAloneWhenThereIsNothingToCollect(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, island, copyOptions{})
	f.pendingCard(t, atraxa, 1, copyOptions{board: BoardConsidering})
	before := f.content(t).Deck.Updated

	summary := f.collectOwn(t, OwnDeckCollect{Boards: []string{BoardMain}})

	assert.Equal(t, CollectSummary{}, summary)
	assert.Equal(t, before, f.content(t).Deck.Updated)
}
