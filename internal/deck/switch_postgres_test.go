//go:build integration

package deck

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f contentFixture) ownedIDs(t *testing.T) []int {
	t.Helper()
	var ids []int
	for _, e := range f.content(t).Entries {
		ids = append(ids, e.OwnedCardIDs...)
	}
	sort.Ints(ids)
	return ids
}

func (f contentFixture) switchToFoil(t *testing.T, cards CardSelection) ReplacementSummary {
	t.Helper()
	summary, err := f.service.SwitchFinish(context.Background(), f.userID, contentDeckID, cards, true)
	require.NoError(t, err)
	return summary
}

func TestSwitchFinish_SwapsCopiesForFreeOnesFirstThenOnesAnotherDeckUses(t *testing.T) {
	f := newContentFixture(t)
	first := f.ownedCopy(t, island, copyOptions{})
	second := f.ownedCopy(t, island, copyOptions{})
	usedElsewhere := f.collectionCopy(t, collectionCopy{printing: island, foil: true})
	f.putInDeck(t, usedElsewhere, otherDeckID)
	free := f.collectionCopy(t, collectionCopy{printing: island, foil: true})

	summary := f.switchToFoil(t, CardSelection{CardIDs: []int{first, second}})

	assert.Equal(t, ReplacementSummary{CardsLinked: 2, CardsRemoved: 2}, summary)
	assert.Equal(t, []int{usedElsewhere, free}, f.ownedIDs(t))
	assert.Equal(t, []entrySummary{{name: island.name, board: BoardMain, foil: true, owned: 2, quantity: 2}}, summarize(f.content(t).Entries))
	assert.Len(t, f.collection(t), 4, "the collection is never modified")
}

func TestSwitchFinish_LeavesTheRestPendingAndNeverUsesLockedStorages(t *testing.T) {
	f := newContentFixture(t)
	pendingID := f.pendingCard(t, island, 3, copyOptions{board: BoardSideboard})
	free := f.collectionCopy(t, collectionCopy{printing: island, foil: true})
	f.collectionCopy(t, collectionCopy{printing: island, foil: true, storageID: f.storage(t, "Vault", true)})
	f.collectionCopy(t, collectionCopy{printing: island, foil: true, proxy: true})

	summary := f.switchToFoil(t, CardSelection{PendingIDs: []int{pendingID}})

	assert.Equal(t, ReplacementSummary{CardsLinked: 1, CardsPending: 2, CardsRemoved: 3}, summary)
	assert.Equal(t, []int{free}, f.ownedIDs(t))
	assert.Equal(t, []entrySummary{{name: island.name, board: BoardSideboard, foil: true, owned: 1, pending: 2, quantity: 3}}, summarize(f.content(t).Entries))
}

func TestSwitchFinish_KeepsTheCommanderAndSecondCommanderInTheirSlots(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	partner := f.ownedCopy(t, tymna, copyOptions{})
	f.setCommanders(t, &commander, nil, &partner, nil)
	foilTymna := f.collectionCopy(t, collectionCopy{printing: tymna, foil: true})

	summary := f.switchToFoil(t, CardSelection{CardIDs: []int{commander, partner}})

	assert.Equal(t, ReplacementSummary{CardsLinked: 1, CardsPending: 1, CardsRemoved: 2}, summary)
	content := f.content(t)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, foil: true, slot: SlotCommander, pending: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, foil: true, slot: SlotPartner, owned: 1, quantity: 1},
	}, summarize(content.Entries))
	assert.Equal(t, &foilTymna, content.Deck.PartnerID)
	assert.Nil(t, content.Deck.CommanderID)
}

func TestSwitchFinish_APendingCommanderThatFindsACopyBecomesTheOwnedCommander(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	partner := f.pendingCard(t, tymna, 1, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)
	foilTymna := f.collectionCopy(t, collectionCopy{printing: tymna, foil: true})

	summary := f.switchToFoil(t, CardSelection{PendingIDs: []int{partner}})

	assert.Equal(t, ReplacementSummary{CardsLinked: 1, CardsRemoved: 1}, summary)
	content := f.content(t)
	assert.Equal(t, &foilTymna, content.Deck.PartnerID)
	assert.Nil(t, content.Deck.PartnerPendingID)
	assert.Equal(t, &commander, content.Deck.CommanderID)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, foil: true, slot: SlotPartner, owned: 1, quantity: 1},
	}, summarize(content.Entries))
}

func TestSwitchFinish_IgnoresCardsAlreadyInThatFinishWithoutTouchingTheDeck(t *testing.T) {
	f := newContentFixture(t)
	foil := f.ownedCopy(t, island, copyOptions{foil: true})
	before := f.state(t)

	summary := f.switchToFoil(t, CardSelection{CardIDs: []int{foil, foil}})

	assert.Zero(t, summary)
	assert.Equal(t, before, f.state(t), "not even the updated date moves")
}

func TestSwitchFinish_RejectsCardsOutsideTheDeck(t *testing.T) {
	f := newContentFixture(t)
	inDeck := f.ownedCopy(t, island, copyOptions{})
	elsewhere := f.collectionCopy(t, collectionCopy{printing: island})
	f.putInDeck(t, elsewhere, otherDeckID)

	f.assertRolledBack(t, func() error {
		_, err := f.service.SwitchFinish(context.Background(), f.userID, contentDeckID, CardSelection{CardIDs: []int{inDeck, elsewhere}}, true)
		assert.ErrorIs(t, err, ErrCardsNotInDeck)
		return err
	})
	f.assertRolledBack(t, func() error {
		_, err := f.service.SwitchFinish(context.Background(), f.userID, contentDeckID, CardSelection{PendingIDs: []int{999}}, true)
		assert.ErrorIs(t, err, ErrCardsNotInDeck)
		return err
	})
}

func TestSwitchFinish_IsIsolatedFromAnotherUser(t *testing.T) {
	f := newContentFixture(t)
	copyID := f.ownedCopy(t, island, copyOptions{})
	bob := seedUser(t, f.db, "bob@example.com")
	var bobsFoil int
	require.NoError(t, f.db.Get(&bobsFoil, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, mana_value)
		VALUES ($1, $2, $3, $4, $5, true, 1) RETURNING id
	`, bob, island.name, island.scryfallID, island.setCode, island.collectorNumber))

	f.assertRolledBack(t, func() error {
		_, err := f.service.SwitchFinish(context.Background(), bob, contentDeckID, CardSelection{CardIDs: []int{copyID}}, true)
		assert.ErrorIs(t, err, ErrNotFound)
		return err
	})

	summary := f.switchToFoil(t, CardSelection{CardIDs: []int{copyID}})
	assert.Equal(t, ReplacementSummary{CardsPending: 1, CardsRemoved: 1}, summary, "Bob's foil Island is not Alice's")
}

func TestSwitchFinish_BumpsTheDecksUpdatedDate(t *testing.T) {
	f := newContentFixture(t)
	copyID := f.ownedCopy(t, island, copyOptions{})
	before := f.content(t).Deck.Updated

	f.switchToFoil(t, CardSelection{CardIDs: []int{copyID}})

	assert.True(t, f.content(t).Deck.Updated.After(before))
}

func TestSwitchFinish_LeavesEverythingAsBeforeWhenRemovingTheOldCopyFails(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, nil)
	f.collectionCopy(t, collectionCopy{printing: atraxa, foil: true})
	pendingID := f.pendingCard(t, island, 2, copyOptions{})
	failNthWrite(t, f.db, "tamiyo.deck_pending_cards", "DELETE", 1)

	f.assertRolledBack(t, func() error {
		_, err := f.service.SwitchFinish(context.Background(), f.userID, contentDeckID, CardSelection{CardIDs: []int{commander}, PendingIDs: []int{pendingID}}, true)
		return err
	})
}

func TestSwitchProxy_KeepsTheFinishAndTheCommanderSlot(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{foil: true})
	f.setCommanders(t, &commander, nil, nil, nil)
	f.collectionCopy(t, collectionCopy{printing: atraxa, proxy: true})
	foilProxy := f.collectionCopy(t, collectionCopy{printing: atraxa, foil: true, proxy: true})

	summary, err := f.service.SwitchProxy(context.Background(), f.userID, contentDeckID, CardSelection{CardIDs: []int{commander}}, true)

	require.NoError(t, err)
	assert.Equal(t, ReplacementSummary{CardsLinked: 1, CardsRemoved: 1}, summary)
	content := f.content(t)
	assert.Equal(t, &foilProxy, content.Deck.CommanderID)
	assert.Equal(t, []entrySummary{{name: atraxa.name, board: BoardMain, foil: true, proxy: true, slot: SlotCommander, owned: 1, quantity: 1}}, summarize(content.Entries))
}

func TestSwitchProxy_APendingProxyTakesRealCopiesAndMergesTheRestIntoThePendingCard(t *testing.T) {
	f := newContentFixture(t)
	proxies := f.pendingCard(t, island, 3, copyOptions{proxy: true})
	real := f.pendingCard(t, island, 1, copyOptions{})
	free := f.collectionCopy(t, collectionCopy{printing: island})
	f.collectionCopy(t, collectionCopy{printing: island, proxy: true})

	summary, err := f.service.SwitchProxy(context.Background(), f.userID, contentDeckID, CardSelection{PendingIDs: []int{proxies, real}}, false)

	require.NoError(t, err)
	assert.Equal(t, ReplacementSummary{CardsLinked: 1, CardsPending: 2, CardsRemoved: 3}, summary)
	entries := f.content(t).Entries
	assert.Equal(t, []entrySummary{{name: island.name, board: BoardMain, owned: 1, pending: 3, quantity: 4}}, summarize(entries))
	assert.Equal(t, []PendingPart{{ID: real, Quantity: 3}}, entries[0].Pending)
	assert.Equal(t, []int{free}, entries[0].OwnedCardIDs)
}

func TestSwitchProxy_LeavesEverythingAsBeforeWhenAPendingCardCantBeAdded(t *testing.T) {
	f := newContentFixture(t)
	first := f.ownedCopy(t, island, copyOptions{})
	second := f.ownedCopy(t, bolt, copyOptions{board: BoardSideboard})
	f.collectionCopy(t, collectionCopy{printing: island, proxy: true})
	failNthWrite(t, f.db, "tamiyo.deck_pending_cards", "INSERT", 1)

	f.assertRolledBack(t, func() error {
		_, err := f.service.SwitchProxy(context.Background(), f.userID, contentDeckID, CardSelection{CardIDs: []int{first, second}}, true)
		return err
	})
}

func (f contentFixture) switchToProxy(t *testing.T, cards CardSelection) ReplacementSummary {
	t.Helper()
	summary, err := f.service.SwitchProxy(context.Background(), f.userID, contentDeckID, cards, true)
	require.NoError(t, err)
	return summary
}

func TestSwitchProxy_SwapsCopiesForFreeProxiesFirstThenOnesAnotherDeckUses(t *testing.T) {
	f := newContentFixture(t)
	first := f.ownedCopy(t, island, copyOptions{})
	second := f.ownedCopy(t, island, copyOptions{})
	usedElsewhere := f.collectionCopy(t, collectionCopy{printing: island, proxy: true})
	f.putInDeck(t, usedElsewhere, otherDeckID)
	free := f.collectionCopy(t, collectionCopy{printing: island, proxy: true})

	summary := f.switchToProxy(t, CardSelection{CardIDs: []int{first, second}})

	assert.Equal(t, ReplacementSummary{CardsLinked: 2, CardsRemoved: 2}, summary)
	assert.Equal(t, []int{usedElsewhere, free}, f.ownedIDs(t))
	assert.Equal(t, []entrySummary{{name: island.name, board: BoardMain, proxy: true, owned: 2, quantity: 2}}, summarize(f.content(t).Entries))
}

func TestSwitchProxy_KeepsTheCommanderAndSecondCommanderInTheirSlots(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	partner := f.ownedCopy(t, tymna, copyOptions{})
	f.setCommanders(t, &commander, nil, &partner, nil)
	proxyAtraxa := f.collectionCopy(t, collectionCopy{printing: atraxa, proxy: true})

	summary := f.switchToProxy(t, CardSelection{CardIDs: []int{commander, partner}})

	assert.Equal(t, ReplacementSummary{CardsLinked: 1, CardsPending: 1, CardsRemoved: 2}, summary)
	content := f.content(t)
	assert.Equal(t, &proxyAtraxa, content.Deck.CommanderID)
	assert.Nil(t, content.Deck.PartnerID)
	assert.NotNil(t, content.Deck.PartnerPendingID, "the second commander stays in its slot as a pending card")
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, proxy: true, slot: SlotCommander, owned: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, proxy: true, slot: SlotPartner, pending: 1, quantity: 1},
	}, summarize(content.Entries))
}

func TestSwitchProxy_APendingCommanderThatFindsACopyBecomesTheOwnedCommander(t *testing.T) {
	f := newContentFixture(t)
	commander := f.pendingCard(t, atraxa, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, nil)
	proxyAtraxa := f.collectionCopy(t, collectionCopy{printing: atraxa, proxy: true})

	summary := f.switchToProxy(t, CardSelection{PendingIDs: []int{commander}})

	assert.Equal(t, ReplacementSummary{CardsLinked: 1, CardsRemoved: 1}, summary)
	content := f.content(t)
	assert.Equal(t, &proxyAtraxa, content.Deck.CommanderID)
	assert.Nil(t, content.Deck.CommanderPendingID)
}

func TestSwitchProxy_IsIsolatedFromAnotherUser(t *testing.T) {
	f := newContentFixture(t)
	copyID := f.ownedCopy(t, island, copyOptions{})
	bob := seedUser(t, f.db, "bob@example.com")
	_, err := f.db.Exec(`
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, proxy, mana_value)
		VALUES ($1, $2, $3, $4, $5, false, true, 1)
	`, bob, island.name, island.scryfallID, island.setCode, island.collectorNumber)
	require.NoError(t, err)

	f.assertRolledBack(t, func() error {
		_, err := f.service.SwitchProxy(context.Background(), bob, contentDeckID, CardSelection{CardIDs: []int{copyID}}, true)
		assert.ErrorIs(t, err, ErrNotFound)
		return err
	})

	summary := f.switchToProxy(t, CardSelection{CardIDs: []int{copyID}})
	assert.Equal(t, ReplacementSummary{CardsPending: 1, CardsRemoved: 1}, summary, "Bob's proxy Island is not Alice's")
}
