//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var islandONE = printing{"Island", "aaaaaaaa-0000-0000-0000-0000000000a1", "one", "263"}

func choice(p printing, foil bool) PrintingChoice {
	return PrintingChoice{ScryfallID: p.scryfallID, SetCode: p.setCode, CollectorNumber: p.collectorNumber, Foil: foil}
}

func repeatChoice(c PrintingChoice, n int) []PrintingChoice {
	out := make([]PrintingChoice, n)
	for i := range out {
		out[i] = c
	}
	return out
}

func (f contentFixture) split(t *testing.T, cards CardSelection, printings []PrintingChoice) SwitchSummary {
	t.Helper()
	summary, err := f.service.SplitPrintings(context.Background(), f.userID, contentDeckID, cards, printings)
	require.NoError(t, err)
	return summary
}

func TestSplitPrintings_KeepsMatchingCopiesAndReplacesTheOthers(t *testing.T) {
	f := newContentFixture(t)
	kept := f.ownedCopy(t, island, copyOptions{board: BoardSideboard})
	leaving := f.ownedCopy(t, island, copyOptions{board: BoardSideboard})
	usedElsewhere := f.collectionCopy(t, collectionCopy{printing: islandONE})
	f.putInDeck(t, usedElsewhere, otherDeckID)
	free := f.collectionCopy(t, collectionCopy{printing: islandONE})

	summary := f.split(t, CardSelection{CardIDs: []int{kept, leaving}}, []PrintingChoice{choice(islandONE, false), choice(island, false)})

	assert.Equal(t, SwitchSummary{CardsLinked: 1, CardsRemoved: 1}, summary)
	assert.Equal(t, []int{kept, free}, f.ownedIDs(t), "the free ONE Island before the one another deck uses")
	for _, e := range f.content(t).Entries {
		assert.Equal(t, BoardSideboard, e.Board)
	}
	assert.Len(t, f.collection(t), 4)
}

func TestSplitPrintings_SplitsAPendingCardAndLeavesTheRestPending(t *testing.T) {
	f := newContentFixture(t)
	pendingID := f.pendingCard(t, island, 10, copyOptions{})
	free := f.collectionCopy(t, collectionCopy{printing: islandONE})

	summary := f.split(t, CardSelection{PendingIDs: []int{pendingID}}, append(repeatChoice(choice(island, false), 7), repeatChoice(choice(islandONE, true), 3)...))

	assert.Equal(t, SwitchSummary{CardsPending: 3, CardsRemoved: 3}, summary, "the free ONE Island isn't foil")
	entries := f.content(t).Entries
	assert.Equal(t, []entrySummary{
		{name: island.name, board: BoardMain, pending: 7, quantity: 7},
		{name: island.name, board: BoardMain, foil: true, pending: 3, quantity: 3},
	}, summarize(entries))
	assert.Equal(t, []PendingPart{{ID: pendingID, Quantity: 7}}, entries[0].Pending)
	assert.Equal(t, islandONE.scryfallID, entries[1].ScryfallID)
	assert.Equal(t, islandONE.setCode, entries[1].SetCode)
	assert.Equal(t, islandONE.collectorNumber, entries[1].CollectorNumber)
	assert.NotContains(t, f.ownedIDs(t), free)
}

func TestSplitPrintings_APendingCardThatFindsAFreeCopyBecomesOwned(t *testing.T) {
	f := newContentFixture(t)
	pendingID := f.pendingCard(t, island, 2, copyOptions{})
	free := f.collectionCopy(t, collectionCopy{printing: islandONE})

	summary := f.split(t, CardSelection{PendingIDs: []int{pendingID}}, repeatChoice(choice(islandONE, false), 2))

	assert.Equal(t, SwitchSummary{CardsLinked: 1, CardsPending: 1, CardsRemoved: 2}, summary)
	entries := f.content(t).Entries
	assert.Equal(t, []entrySummary{{name: island.name, board: BoardMain, owned: 1, pending: 1, quantity: 2}}, summarize(entries))
	assert.Equal(t, []int{free}, entries[0].OwnedCardIDs)
	require.Len(t, entries[0].Pending, 1)
	assert.NotEqual(t, pendingID, entries[0].Pending[0].ID, "the NEO pending card is gone")
}

func TestSplitPrintings_AskingForThePrintingsTheCopiesHaveChangesNothing(t *testing.T) {
	f := newContentFixture(t)
	copyID := f.ownedCopy(t, island, copyOptions{})
	pendingID := f.pendingCard(t, island, 2, copyOptions{})
	before := f.state(t)

	summary := f.split(t, CardSelection{CardIDs: []int{copyID}, PendingIDs: []int{pendingID}}, repeatChoice(choice(island, false), 3))

	assert.Zero(t, summary)
	assert.Equal(t, before, f.state(t))
}

func TestSplitPrintings_RejectsMismatchesAndTheCommander(t *testing.T) {
	f := newContentFixture(t)
	first := f.ownedCopy(t, island, copyOptions{})
	second := f.ownedCopy(t, island, copyOptions{})
	sideboard := f.ownedCopy(t, island, copyOptions{board: BoardSideboard})
	boltCopy := f.ownedCopy(t, bolt, copyOptions{})
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	partner := f.pendingCard(t, tymna, 1, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)
	elsewhere := f.collectionCopy(t, collectionCopy{printing: island})
	one := choice(islandONE, false)

	cases := []struct {
		name      string
		cards     CardSelection
		printings []PrintingChoice
		err       error
	}{
		{"more printings than copies", CardSelection{CardIDs: []int{first, second}}, repeatChoice(one, 3), ErrSplitMismatch},
		{"two cards", CardSelection{CardIDs: []int{first, boltCopy}}, repeatChoice(one, 2), ErrSplitMismatch},
		{"two boards", CardSelection{CardIDs: []int{first, sideboard}}, repeatChoice(one, 2), ErrSplitMismatch},
		{"the commander", CardSelection{CardIDs: []int{commander}}, repeatChoice(one, 1), ErrSplitMismatch},
		{"the second commander", CardSelection{PendingIDs: []int{partner}}, repeatChoice(one, 1), ErrSplitMismatch},
		{"a copy outside the deck", CardSelection{CardIDs: []int{first, elsewhere}}, repeatChoice(one, 2), ErrCardsNotInDeck},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.assertRolledBack(t, func() error {
				_, err := f.service.SplitPrintings(context.Background(), f.userID, contentDeckID, tc.cards, tc.printings)
				assert.ErrorIs(t, err, tc.err)
				return err
			})
		})
	}
}

func TestSplitPrintings_IsIsolatedFromAnotherUser(t *testing.T) {
	f := newContentFixture(t)
	copyID := f.ownedCopy(t, island, copyOptions{})
	bob := seedUser(t, f.db, "bob@example.com")

	f.assertRolledBack(t, func() error {
		_, err := f.service.SplitPrintings(context.Background(), bob, contentDeckID, CardSelection{CardIDs: []int{copyID}}, repeatChoice(choice(islandONE, false), 1))
		assert.ErrorIs(t, err, ErrNotFound)
		return err
	})
}

func TestSplitPrintings_BumpsTheDecksUpdatedDate(t *testing.T) {
	f := newContentFixture(t)
	copyID := f.ownedCopy(t, island, copyOptions{})
	before := f.content(t).Deck.Updated

	f.split(t, CardSelection{CardIDs: []int{copyID}}, repeatChoice(choice(islandONE, false), 1))

	assert.True(t, f.content(t).Deck.Updated.After(before))
}

func TestSplitPrintings_AppliesEveryPrintingOrNone(t *testing.T) {
	f := newContentFixture(t)
	first := f.ownedCopy(t, island, copyOptions{})
	second := f.ownedCopy(t, island, copyOptions{})
	pendingID := f.pendingCard(t, island, 2, copyOptions{})
	f.collectionCopy(t, collectionCopy{printing: islandONE})
	failNthWrite(t, f.db, "tamiyo.deck_pending_cards", "INSERT", 1)

	f.assertRolledBack(t, func() error {
		_, err := f.service.SplitPrintings(context.Background(), f.userID, contentDeckID,
			CardSelection{CardIDs: []int{first, second}, PendingIDs: []int{pendingID}},
			append(repeatChoice(choice(islandONE, false), 2), repeatChoice(choice(islandONE, true), 2)...))
		return err
	})
}

func TestSplitPrintings_TakesFreeCopiesBeforeOnesAnotherDeckUses(t *testing.T) {
	f := newContentFixture(t)
	first := f.ownedCopy(t, island, copyOptions{})
	second := f.ownedCopy(t, island, copyOptions{})
	usedElsewhere := f.collectionCopy(t, collectionCopy{printing: islandONE})
	f.putInDeck(t, usedElsewhere, otherDeckID)
	free := f.collectionCopy(t, collectionCopy{printing: islandONE})

	summary := f.split(t, CardSelection{CardIDs: []int{first, second}}, repeatChoice(choice(islandONE, false), 2))

	assert.Equal(t, SwitchSummary{CardsLinked: 2, CardsRemoved: 2}, summary)
	assert.Equal(t, []int{usedElsewhere, free}, f.ownedIDs(t))
	assert.Len(t, f.collection(t), 4, "the collection is never modified")
}
