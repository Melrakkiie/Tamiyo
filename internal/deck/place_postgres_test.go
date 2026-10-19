//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (p printing) stored() Printing {
	return Printing{Name: p.name, ScryfallID: p.scryfallID, SetCode: p.setCode, CollectorNumber: p.collectorNumber, ManaValue: 1}
}

func line(p printing, quantity int) PlacementLine {
	return PlacementLine{Printing: p.stored(), Board: BoardMain, Quantity: quantity}
}

func (f contentFixture) place(t *testing.T, plan PlacementPlan) PlacementSummary {
	t.Helper()
	summary, err := f.service.PlaceCards(context.Background(), f.userID, contentDeckID, plan)
	require.NoError(t, err)
	return summary
}

func TestPlaceCards_UsesFreeOwnedCopiesFirstAndAddsPendingCardsForTheRest(t *testing.T) {
	f := newContentFixture(t)
	f.collectionCopy(t, collectionCopy{printing: bolt})
	f.collectionCopy(t, collectionCopy{printing: bolt})
	sideboard := line(island, 2)
	sideboard.Board = BoardSideboard

	summary := f.place(t, PlacementPlan{Lines: []PlacementLine{line(bolt, 3), sideboard}})

	assert.Equal(t, PlacementSummary{CardsLinked: 2, CardsPending: 3}, summary)
	assert.Equal(t, []entrySummary{
		{name: bolt.name, board: BoardMain, owned: 2, pending: 1, quantity: 3},
		{name: island.name, board: BoardSideboard, pending: 2, quantity: 2},
	}, summarize(f.content(t).Entries))
	assert.Len(t, f.collection(t), 2, "placing cards creates no collection card")
}

func commanderLine(p printing) PlacementLine {
	l := line(p, 1)
	l.Commander = true
	return l
}

func TestPlaceCards_SetsAnOwnedCommanderAndAPendingSecondCommander(t *testing.T) {
	f := newContentFixture(t)
	owned := f.collectionCopy(t, collectionCopy{printing: atraxa})

	f.place(t, PlacementPlan{Lines: []PlacementLine{commanderLine(atraxa), commanderLine(tymna), line(bolt, 1)}})

	content := f.content(t)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, slot: SlotPartner, pending: 1, quantity: 1},
		{name: bolt.name, board: BoardMain, pending: 1, quantity: 1},
	}, summarize(content.Entries))
	assert.Equal(t, &owned, content.Deck.CommanderID)
	require.NotNil(t, content.Deck.PartnerPendingID)
}

func TestPlaceCards_SetsAPendingCommanderApartFromAPendingCardOfTheSamePrinting(t *testing.T) {
	f := newContentFixture(t)
	f.pendingCard(t, atraxa, 2, copyOptions{})

	f.place(t, PlacementPlan{Lines: []PlacementLine{commanderLine(atraxa)}})

	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, pending: 1, quantity: 1},
		{name: atraxa.name, board: BoardMain, pending: 2, quantity: 2},
	}, summarize(f.content(t).Entries))
}

func TestPlaceCards_KeepsTheCommanderOfADeckThatHasOne(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, nil)

	f.place(t, PlacementPlan{Lines: []PlacementLine{commanderLine(tymna)}})

	content := f.content(t)
	assert.Equal(t, &commander, content.Deck.CommanderID)
	assert.Nil(t, content.Deck.PartnerID)
	assert.Nil(t, content.Deck.PartnerPendingID)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, pending: 1, quantity: 1},
	}, summarize(content.Entries))
}

func TestPlaceCards_BumpsTheDecksUpdatedDateOnce(t *testing.T) {
	f := newContentFixture(t)
	before := f.content(t).Deck.Updated

	f.place(t, PlacementPlan{Lines: []PlacementLine{line(bolt, 1), line(island, 1)}})

	assert.True(t, f.content(t).Deck.Updated.After(before))
}

func TestPlaceCards_IsIsolatedFromAnotherUser(t *testing.T) {
	f := newContentFixture(t)
	bob := contentFixture{db: f.db, repo: f.repo, service: f.service, userID: seedUser(t, f.db, "bob@example.com")}
	bob.collectionCopy(t, collectionCopy{printing: bolt})

	f.assertRolledBack(t, func() error {
		_, err := f.service.PlaceCards(context.Background(), bob.userID, contentDeckID, PlacementPlan{Lines: []PlacementLine{line(bolt, 1)}})
		assert.ErrorIs(t, err, ErrNotFound)
		return err
	})

	summary := f.place(t, PlacementPlan{Lines: []PlacementLine{line(bolt, 1)}})
	assert.Equal(t, PlacementSummary{CardsPending: 1}, summary, "bob's copy is not alice's")
}

func TestPlaceCards_LeavesTheDeckAsBeforeWhenAWriteFails(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, nil)
	f.pendingCard(t, island, 1, copyOptions{})
	f.collectionCopy(t, collectionCopy{printing: bolt})
	failNthWrite(t, f.db, "tamiyo.deck_pending_cards", "INSERT", 2)

	f.assertRolledBack(t, func() error {
		_, err := f.service.PlaceCards(context.Background(), f.userID, contentDeckID, PlacementPlan{
			Lines: []PlacementLine{line(bolt, 2), line(island, 3), commanderLine(tymna), line(atraxa, 1)},
			Tags:  []TaggedCard{{Name: bolt.name, Tags: []string{"Burn"}}},
		})
		return err
	})
	assert.Empty(t, f.tags(t))
}

func TestPlaceCards_LeavesTheDeckAsBeforeWhenTaggingFails(t *testing.T) {
	f := newContentFixture(t)
	f.collectionCopy(t, collectionCopy{printing: bolt})
	failNthWrite(t, f.db, "tamiyo.deck_card_tags", "INSERT", 1)

	f.assertRolledBack(t, func() error {
		_, err := f.service.PlaceCards(context.Background(), f.userID, contentDeckID, PlacementPlan{
			Lines: []PlacementLine{commanderLine(atraxa), line(bolt, 1)},
			Tags:  []TaggedCard{{Name: bolt.name, Tags: []string{"Burn"}}},
		})
		return err
	})
}

func (f contentFixture) tags(t *testing.T) []TaggedCard {
	t.Helper()
	tags, err := f.service.GetCardTags(context.Background(), f.userID, contentDeckID)
	require.NoError(t, err)
	return tags.Cards
}

func TestPlaceCards_AddsTheTagsToThoseTheCardsAlreadyHave(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{})
	_, err := f.service.SetCardTags(context.Background(), f.userID, contentDeckID, bolt.name, []string{"Burn"})
	require.NoError(t, err)

	summary := f.place(t, PlacementPlan{
		Lines: []PlacementLine{line(island, 1)},
		Tags: []TaggedCard{
			{Name: "lightning bolt", Tags: []string{"burn", "Removal"}},
			{Name: island.name, Tags: []string{"Land"}},
		},
	})

	assert.Empty(t, summary.SkippedTags)
	assert.Equal(t, []TaggedCard{
		{Name: island.name, Tags: []string{"Land"}},
		{Name: bolt.name, Tags: []string{"Burn", "Removal"}},
	}, f.tags(t))
}

func TestPlaceCards_ReportsTagsItCannotApplyAndPlacesTheCards(t *testing.T) {
	f := newContentFixture(t)

	summary := f.place(t, PlacementPlan{
		Lines: []PlacementLine{line(bolt, 1)},
		Tags: []TaggedCard{
			{Name: island.name, Tags: []string{"Land"}},
			{Name: bolt.name, Tags: []string{"   "}},
		},
	})

	assert.Equal(t, []SkippedTags{
		{CardName: island.name, Err: ErrCardNotInDeck},
		{CardName: bolt.name, Err: ErrInvalidTag},
	}, summary.SkippedTags)
	assert.Empty(t, f.tags(t))
	assert.Equal(t, 1, summary.CardsPending)
}

func TestPlaceCards_SkipsCopiesInStoragesLockedAgainstTheDeck(t *testing.T) {
	f := newContentFixture(t)
	f.collectionCopy(t, collectionCopy{printing: bolt, storageID: f.storage(t, "Trades", true)})
	open := f.collectionCopy(t, collectionCopy{printing: bolt, storageID: f.storage(t, "Cube", true, contentDeckID)})

	summary := f.place(t, PlacementPlan{Lines: []PlacementLine{line(bolt, 2)}})

	assert.Equal(t, PlacementSummary{CardsLinked: 1, CardsPending: 1}, summary)
	assert.Equal(t, []int{open}, f.content(t).Entries[0].OwnedCardIDs)
}

func TestPlaceCards_UsesLockedStoragesLastWhenAskedTo(t *testing.T) {
	f := newContentFixture(t)
	locked := f.collectionCopy(t, collectionCopy{printing: bolt, storageID: f.storage(t, "Trades", true)})
	free := f.collectionCopy(t, collectionCopy{printing: bolt})

	f.place(t, PlacementPlan{Lines: []PlacementLine{line(bolt, 1)}, IncludeLockedStorages: true})
	assert.Equal(t, []int{free}, f.content(t).Entries[0].OwnedCardIDs)

	f.place(t, PlacementPlan{Lines: []PlacementLine{line(bolt, 1)}, IncludeLockedStorages: true})
	assert.ElementsMatch(t, []int{free, locked}, f.content(t).Entries[0].OwnedCardIDs)
}

func TestPlaceCards_PrefersCopiesInNoDeckThenTheRequestedFinish(t *testing.T) {
	f := newContentFixture(t)
	inOtherDeck := f.collectionCopy(t, collectionCopy{printing: bolt, foil: true})
	f.putInDeck(t, inOtherDeck, otherDeckID)
	alreadyThere := f.ownedCopy(t, bolt, copyOptions{foil: true})
	nonFoil := f.collectionCopy(t, collectionCopy{printing: bolt})
	foil := f.collectionCopy(t, collectionCopy{printing: bolt, foil: true})
	foilLine := line(bolt, 1)
	foilLine.Foil = true

	summary := f.place(t, PlacementPlan{Lines: []PlacementLine{foilLine, foilLine}})

	assert.Equal(t, PlacementSummary{CardsLinked: 2}, summary)
	var linked []int
	for _, e := range f.content(t).Entries {
		linked = append(linked, e.OwnedCardIDs...)
	}
	assert.ElementsMatch(t, []int{alreadyThere, foil, nonFoil}, linked)
}

func TestPlaceCards_MatchesTheRequestedProxyState(t *testing.T) {
	f := newContentFixture(t)
	f.collectionCopy(t, collectionCopy{printing: bolt, proxy: true})
	real := f.collectionCopy(t, collectionCopy{printing: bolt})
	proxyLine := line(bolt, 1)
	proxyLine.Proxy = ProxyOnly
	realLine := line(bolt, 1)
	realLine.Proxy = RealOnly

	f.place(t, PlacementPlan{Lines: []PlacementLine{realLine}})
	summary := f.place(t, PlacementPlan{Lines: []PlacementLine{realLine, proxyLine}})

	assert.Equal(t, PlacementSummary{CardsLinked: 1, CardsPending: 1}, summary)
	entries := f.content(t).Entries
	assert.Equal(t, []entrySummary{
		{name: bolt.name, board: BoardMain, owned: 1, pending: 1, quantity: 2},
		{name: bolt.name, board: BoardMain, proxy: true, owned: 1, quantity: 1},
	}, summarize(entries))
	assert.Equal(t, []int{real}, entries[0].OwnedCardIDs)
}

func TestPlaceCards_TakesCopiesOfAnyPrintingOfTheCardWhenAsked(t *testing.T) {
	f := newContentFixture(t)
	otherPrinting := printing{bolt.name, "1c3c3e33-7f1a-4d1a-9b2b-4f5e6d7c8b9a", "lea", "161"}
	copyID := f.collectionCopy(t, collectionCopy{printing: otherPrinting})
	anyBolt := line(bolt, 2)
	anyBolt.AnyPrinting = true

	f.place(t, PlacementPlan{Lines: []PlacementLine{anyBolt}})

	entries := f.content(t).Entries
	require.Len(t, entries, 2)
	assert.Equal(t, bolt.setCode, entries[0].SetCode)
	assert.Equal(t, 1, entries[0].PendingQuantity())
	assert.Equal(t, otherPrinting.setCode, entries[1].SetCode)
	assert.Equal(t, []int{copyID}, entries[1].OwnedCardIDs)
}
