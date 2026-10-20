//go:build integration

package deck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaceCards_AppliesRemovalsPlacementsAndTagReplacementTogether(t *testing.T) {
	f := newContentFixture(t)
	dropped := f.ownedCopy(t, island, copyOptions{})
	f.ownedCopy(t, bolt, copyOptions{})
	gone := f.pendingCard(t, tymna, 1, copyOptions{})
	lowered := f.pendingCard(t, atraxa, 3, copyOptions{})
	_, err := f.service.SetCardTags(context.Background(), f.userID, contentDeckID, bolt.name, []string{"Burn"})
	require.NoError(t, err)
	before := f.content(t).Deck.Updated

	summary := f.place(t, PlacementPlan{
		Lines:           []PlacementLine{line(tymna, 2)},
		RemoveCards:     []int{dropped},
		PendingRemovals: []PendingRemoval{{ID: gone, Count: 1}, {ID: lowered, Count: 2}},
		ReplaceTags:     []TaggedCard{{Name: bolt.name, Tags: []string{"Removal"}}},
	})

	assert.Equal(t, PlacementSummary{CardsPending: 2}, summary)
	content := f.content(t)
	assert.Equal(t, []entrySummary{
		{name: atraxa.name, board: BoardMain, pending: 1, quantity: 1},
		{name: bolt.name, board: BoardMain, owned: 1, quantity: 1},
		{name: tymna.name, board: BoardMain, pending: 2, quantity: 2},
	}, summarize(content.Entries))
	assert.True(t, content.Deck.Updated.After(before))
	assert.Equal(t, []TaggedCard{{Name: bolt.name, Tags: []string{"Removal"}}}, f.tags(t))
	assert.Equal(t, []collectionCard{
		{name: island.name, scryfallID: island.scryfallID},
		{name: bolt.name, scryfallID: bolt.scryfallID},
	}, f.collection(t), "a removed copy stays in the collection")
}

func TestPlaceCards_ClearsTheCommanderSlotOfARemovedCopy(t *testing.T) {
	f := newContentFixture(t)
	first := f.ownedCopy(t, atraxa, copyOptions{})
	second := f.ownedCopy(t, tymna, copyOptions{})
	f.setCommanders(t, &first, nil, &second, nil)

	f.place(t, PlacementPlan{RemoveCards: []int{first}})

	content := f.content(t)
	assert.Equal(t, &second, content.Deck.CommanderID, "the partner moves up")
	assert.Nil(t, content.Deck.PartnerID)
	assert.Equal(t, []entrySummary{{name: tymna.name, board: BoardMain, slot: SlotCommander, owned: 1, quantity: 1}}, summarize(content.Entries))
}

func TestPlaceCards_ClearsTheCommanderSlotOfARemovedPendingCard(t *testing.T) {
	f := newContentFixture(t)
	pending := f.pendingCard(t, atraxa, 1, copyOptions{})
	f.setCommanders(t, nil, &pending, nil, nil)

	f.place(t, PlacementPlan{PendingRemovals: []PendingRemoval{{ID: pending, Count: 1}}})

	content := f.content(t)
	assert.Nil(t, content.Deck.CommanderPendingID)
	assert.Empty(t, content.Entries)
}

func TestPlaceCards_ReportsReplacementTagsOfCardsNotInTheDeck(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{})

	summary := f.place(t, PlacementPlan{ReplaceTags: []TaggedCard{
		{Name: island.name, Tags: []string{"Land"}},
		{Name: bolt.name, Tags: []string{"Burn"}},
	}})

	assert.Equal(t, []SkippedTags{{CardName: island.name, Err: ErrCardNotInDeck}}, summary.SkippedTags)
	assert.Equal(t, []TaggedCard{{Name: bolt.name, Tags: []string{"Burn"}}}, f.tags(t))
}

func TestPlaceCards_RemovesNothingOfAnotherUsersDeck(t *testing.T) {
	f := newContentFixture(t)
	copyID := f.ownedCopy(t, bolt, copyOptions{})
	pendingID := f.pendingCard(t, island, 2, copyOptions{})
	bobID := seedUser(t, f.db, "bob@example.com")

	f.assertRolledBack(t, func() error {
		_, err := f.service.PlaceCards(context.Background(), bobID, contentDeckID, PlacementPlan{
			RemoveCards:     []int{copyID},
			PendingRemovals: []PendingRemoval{{ID: pendingID, Count: 2}},
		})
		assert.ErrorIs(t, err, ErrNotFound)
		return err
	})
}

func TestPlaceCards_RejectsRemovalsOfCardsNotInTheDeck(t *testing.T) {
	f := newContentFixture(t)
	f.ownedCopy(t, bolt, copyOptions{})
	other := f.collectionCopy(t, collectionCopy{printing: island})

	f.assertRolledBack(t, func() error {
		_, err := f.service.PlaceCards(context.Background(), f.userID, contentDeckID, PlacementPlan{
			Lines:       []PlacementLine{line(atraxa, 1)},
			RemoveCards: []int{other},
		})
		return err
	})
}

func TestPlaceCards_LeavesTheDeckAsBeforeWhenARemovalFails(t *testing.T) {
	f := newContentFixture(t)
	commander := f.ownedCopy(t, atraxa, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, nil)
	second := f.ownedCopy(t, bolt, copyOptions{})
	pending := f.pendingCard(t, island, 3, copyOptions{})
	failNthWrite(t, f.db, "tamiyo.deck_pending_cards", "UPDATE", 1)

	f.assertRolledBack(t, func() error {
		_, err := f.service.PlaceCards(context.Background(), f.userID, contentDeckID, PlacementPlan{
			Lines:           []PlacementLine{line(tymna, 1)},
			RemoveCards:     []int{commander, second},
			PendingRemovals: []PendingRemoval{{ID: pending, Count: 2}},
			ReplaceTags:     []TaggedCard{{Name: island.name, Tags: []string{"Land"}}},
		})
		return err
	})
}

func TestPlaceCards_LeavesTheDeckAsBeforeWhenTheTagReplacementFails(t *testing.T) {
	f := newContentFixture(t)
	copyID := f.ownedCopy(t, bolt, copyOptions{})
	f.ownedCopy(t, island, copyOptions{})
	failNthWrite(t, f.db, "tamiyo.deck_card_tags", "INSERT", 1)

	f.assertRolledBack(t, func() error {
		_, err := f.service.PlaceCards(context.Background(), f.userID, contentDeckID, PlacementPlan{
			RemoveCards: []int{copyID},
			ReplaceTags: []TaggedCard{{Name: island.name, Tags: []string{"Land"}}},
		})
		return err
	})
}

func TestPlaceCards_RemovesFromAPendingCardWhatItHoldsNow(t *testing.T) {
	f := newContentFixture(t)
	pending := f.pendingCard(t, island, 3, copyOptions{})
	_, err := f.service.PlaceCards(context.Background(), f.userID, contentDeckID, PlacementPlan{Lines: []PlacementLine{line(island, 2)}})
	require.NoError(t, err)

	f.place(t, PlacementPlan{PendingRemovals: []PendingRemoval{{ID: pending, Count: 1}}})

	assert.Equal(t, []entrySummary{{name: island.name, board: BoardMain, pending: 4, quantity: 4}}, summarize(f.content(t).Entries), "a copy added since the plan was made stays")
}

func TestPlaceCards_RejectsRemovingMoreFromAPendingCardThanItHolds(t *testing.T) {
	f := newContentFixture(t)
	pending := f.pendingCard(t, island, 1, copyOptions{})

	f.assertRolledBack(t, func() error {
		_, err := f.service.PlaceCards(context.Background(), f.userID, contentDeckID, PlacementPlan{
			PendingRemovals: []PendingRemoval{{ID: pending, Count: 2}},
		})
		assert.ErrorIs(t, err, ErrCardNotInDeck)
		return err
	})
}
