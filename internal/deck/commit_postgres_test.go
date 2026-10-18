//go:build integration

package deck

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f contentFixture) commit(t *testing.T, req PendingCommit) PendingCommitSummary {
	t.Helper()
	summary, err := f.service.CommitPendingCards(context.Background(), f.userID, contentDeckID, req)
	require.NoError(t, err)
	return summary
}

func TestCommitPendingCards_CreatesEveryNonConsideringPendingCardAndPutsItInTheDeckOnItsBoard(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 2, copyOptions{})
	f.pendingCard(t, island, 1, copyOptions{board: BoardSideboard})
	f.pendingCard(t, atraxa, 1, copyOptions{board: BoardConsidering})

	summary := f.commit(t, PendingCommit{})

	assert.Equal(t, PendingCommitSummary{CardsCreated: 3}, summary)
	assert.Equal(t, []entrySummary{
		{name: bolt.name, board: BoardMain, owned: 2, quantity: 2},
		{name: island.name, board: BoardSideboard, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardConsidering, pending: 1, quantity: 1},
	}, summarize(f.content(t).Entries))
	assert.Equal(t, []collectionCard{
		{name: island.name, scryfallID: island.scryfallID},
		{name: bolt.name, scryfallID: bolt.scryfallID},
		{name: bolt.name, scryfallID: bolt.scryfallID},
	}, f.collection(t))
}

func TestCommitPendingCards_CommittingPartOfAPendingCardLowersItsQuantity(t *testing.T) {
	f := newContentFixture(t)
	pendingID := f.pendingCard(t, island, 5, copyOptions{})

	summary := f.commit(t, PendingCommit{PendingID: &pendingID, Quantity: intPtr(2)})

	assert.Equal(t, PendingCommitSummary{CardsCreated: 2}, summary)
	entries := f.content(t).Entries
	assert.Equal(t, []entrySummary{{name: island.name, board: BoardMain, owned: 2, pending: 3, quantity: 5}}, summarize(entries))
	assert.Equal(t, []PendingPart{{ID: pendingID, Quantity: 3}}, entries[0].Pending)
	assert.Len(t, f.collection(t), 2)
}

func TestCommitPendingCards_CommitsAQuantityAboveThePendingCardsAsTheWholeCard(t *testing.T) {
	f := newContentFixture(t)
	pendingID := f.pendingCard(t, island, 2, copyOptions{})

	summary := f.commit(t, PendingCommit{PendingID: &pendingID, Quantity: intPtr(5)})

	assert.Equal(t, PendingCommitSummary{CardsCreated: 2}, summary)
	assert.Equal(t, []entrySummary{{name: island.name, board: BoardMain, owned: 2, quantity: 2}}, summarize(f.content(t).Entries))
}

func TestCommitPendingCards_CommitsOnePendingCardOfTheConsideringBoard(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})
	considered := f.pendingCard(t, atraxa, 1, copyOptions{board: BoardConsidering, foil: true, proxy: true})

	f.commit(t, PendingCommit{PendingID: &considered})

	assert.Equal(t, []entrySummary{
		{name: bolt.name, board: BoardMain, pending: 1, quantity: 1},
		{name: atraxa.name, board: BoardConsidering, foil: true, proxy: true, owned: 1, quantity: 1},
	}, summarize(f.content(t).Entries))
	assert.Equal(t, []collectionCard{{name: atraxa.name, scryfallID: atraxa.scryfallID, foil: true, proxy: true}}, f.collection(t))
}

func TestCommitPendingCards_MovesThePendingCommanderToItsFirstCopy(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 2, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, nil)

	f.commit(t, PendingCommit{})

	content := f.content(t)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, owned: 1, quantity: 1},
	}, summarize(content.Entries))
	assert.Nil(t, content.Deck.CommanderPendingID)
	require.NotNil(t, content.Deck.CommanderID)
	assert.Equal(t, []int{*content.Deck.CommanderID}, content.Entries[0].OwnedCardIDs)
}

func TestCommitPendingCards_KeepsTheCommanderWhenCommittingOneCopyOfIt(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 3, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, nil)

	f.commit(t, PendingCommit{PendingID: &commander, Quantity: intPtr(1)})

	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, pending: 2, quantity: 2},
	}, summarize(f.content(t).Entries))
}

func TestCommitPendingCards_KeepsAPendingSecondCommanderAsSecondCommander(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	partner := f.pendingCard(t, tymna, 1, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)

	f.commit(t, PendingCommit{})

	content := f.content(t)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, slot: SlotPartner, owned: 1, quantity: 1},
	}, summarize(content.Entries))
	assert.Equal(t, &commander, content.Deck.CommanderID)
	assert.Nil(t, content.Deck.PartnerPendingID)
}

func TestCommitPendingCards_CommitsBothPendingCommanders(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 1, copyOptions{})
	partner := f.pendingCard(t, tymna, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, &partner)

	f.commit(t, PendingCommit{})

	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, slot: SlotPartner, owned: 1, quantity: 1},
	}, summarize(f.content(t).Entries))
}

func TestCommitPendingCards_PutsTheCopiesInTheTargetStorage(t *testing.T) {
	f := newContentFixture(t)
	box := f.storage(t, "Box")
	f.pendingCard(t, bolt, 2, copyOptions{})

	f.commit(t, PendingCommit{StorageID: &box})

	assert.Equal(t, []collectionCard{
		{name: bolt.name, scryfallID: bolt.scryfallID, storageID: &box},
		{name: bolt.name, scryfallID: bolt.scryfallID, storageID: &box},
	}, f.collection(t))
}

func TestCommitPendingCards_BumpsTheDecksUpdatedDateOnceAtTheCommitTime(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 2, copyOptions{})
	before := f.content(t).Deck.Updated

	f.commit(t, PendingCommit{})

	updated := f.content(t).Deck.Updated
	assert.True(t, updated.After(before))
	var added []time.Time
	require.NoError(t, f.db.Select(&added, `SELECT added FROM tamiyo.card_deck WHERE deck_id = $1`, contentDeckID))
	require.Len(t, added, 2)
	for _, a := range added {
		assert.True(t, a.Equal(updated), "the deck's updated date is the commit's time")
	}
}

func TestCommitPendingCards_RejectsAnUnknownPendingCard(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})

	f.assertRolledBack(t, func() error {
		_, err := f.service.CommitPendingCards(context.Background(), f.userID, contentDeckID, PendingCommit{PendingID: intPtr(999)})
		assert.ErrorIs(t, err, ErrPendingCardNotFound)
		return err
	})
}

func TestCommitPendingCards_RejectsAnUnknownStorage(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})

	f.assertRolledBack(t, func() error {
		_, err := f.service.CommitPendingCards(context.Background(), f.userID, contentDeckID, PendingCommit{StorageID: intPtr(999)})
		assert.ErrorIs(t, err, ErrTargetStorageNotFound)
		return err
	})
}

func TestCommitPendingCards_IsIsolatedFromAnotherUser(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 1, copyOptions{})
	bob := seedUser(t, f.db, "bob@example.com")
	var bobsStorage int
	require.NoError(t, f.db.Get(&bobsStorage, `INSERT INTO tamiyo.storage (user_id, name, type) VALUES ($1, 'Bob box', 'box') RETURNING id`, bob))

	f.assertRolledBack(t, func() error {
		_, err := f.service.CommitPendingCards(context.Background(), bob, contentDeckID, PendingCommit{})
		assert.ErrorIs(t, err, ErrNotFound)
		return err
	})
	f.assertRolledBack(t, func() error {
		_, err := f.service.CommitPendingCards(context.Background(), f.userID, contentDeckID, PendingCommit{StorageID: &bobsStorage})
		assert.ErrorIs(t, err, ErrTargetStorageNotFound)
		return err
	})
}

func TestCommitPendingCards_LeavesEverythingAsBeforeWhenTheThirdCopyFails(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, nil)
	f.pendingCard(t, bolt, 3, copyOptions{})
	f.pendingCard(t, island, 2, copyOptions{board: BoardSideboard})
	failNthWrite(t, f.db, "tamiyo.cards", "INSERT", 3)

	f.assertRolledBack(t, func() error {
		_, err := f.service.CommitPendingCards(context.Background(), f.userID, contentDeckID, PendingCommit{})
		return err
	})
}

func TestCommitPendingCards_RetryingAfterAFailureCreatesNoDuplicate(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, bolt, 3, copyOptions{})
	failNthWrite(t, f.db, "tamiyo.deck_pending_cards", "DELETE", 1)
	_, err := f.service.CommitPendingCards(context.Background(), f.userID, contentDeckID, PendingCommit{})
	require.Error(t, err)

	summary := f.commit(t, PendingCommit{})

	assert.Equal(t, PendingCommitSummary{CardsCreated: 3}, summary)
	assert.Len(t, f.collection(t), 3)
	assert.Equal(t, []entrySummary{{name: bolt.name, board: BoardMain, owned: 3, quantity: 3}}, summarize(f.content(t).Entries))
}
