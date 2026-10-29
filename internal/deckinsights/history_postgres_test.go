//go:build integration

package deckinsights

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

var (
	historyIsland = printing{ScryfallID: "11111111-1111-1111-1111-111111111111", Name: "Island", SetCode: "lea", CollectorNumber: "1", TypeLine: "Basic Land — Island"}
	historyBolt   = printing{ScryfallID: "22222222-2222-2222-2222-222222222222", Name: "Lightning Bolt", SetCode: "lea", CollectorNumber: "2", TypeLine: "Instant"}
)

type historyFixture struct {
	insightsFixture
	decks   *deck.Service
	cards   *card.Service
	service *Service
}

func newHistoryFixture(t *testing.T) historyFixture {
	t.Helper()
	f := historyFixture{insightsFixture: newInsightsFixture(t, "standard")}
	seedPrintings(t, f.db, historyIsland, historyBolt)
	f.decks = deck.NewService(deck.NewPostgresRepository(f.db))
	f.cards = card.NewService(card.NewPostgresRepository(f.db))
	f.service = NewService(f.decks, NewPostgresRepository(f.db), nil)
	return f
}

func (f historyFixture) place(t *testing.T, deckID string, p printing, board string, quantity int) {
	t.Helper()
	line := deck.PlacementLine{
		Printing: deck.Printing{Name: p.Name, ScryfallID: p.ScryfallID, SetCode: p.SetCode, CollectorNumber: p.CollectorNumber, ManaValue: 1},
		Board:    board, Quantity: quantity,
	}
	_, err := f.decks.PlaceCards(context.Background(), f.userID, deckID, deck.PlacementPlan{Lines: []deck.PlacementLine{line}})
	require.NoError(t, err)
}

func (f historyFixture) history(t *testing.T) DeckHistory {
	t.Helper()
	h, err := f.service.GetDeckHistory(context.Background(), f.userID, f.deckID, deck.ReachOwn)
	require.NoError(t, err)
	return h
}

// changes flattens today's history to "board name" -> count.
func today(h DeckHistory) map[string]int {
	out := map[string]int{}
	for _, d := range h.Days {
		for _, b := range d.Boards {
			for _, c := range b.Changes {
				out[b.Board+" "+c.Name] = c.Count
			}
		}
	}
	return out
}

func TestPostgresRepository_GetDeckHistory_EmptyDeckReturnsNoDays(t *testing.T) {
	f := newHistoryFixture(t)

	assert.Equal(t, DeckHistory{Days: []HistoryDay{}}, f.history(t))
}

func TestPostgresRepository_GetDeckHistory_PlacingGivesAdditionsOnTheirBoard(t *testing.T) {
	f := newHistoryFixture(t)
	f.collectionCopy(t, historyBolt, copyOptions{})

	f.place(t, f.deckID, historyBolt, deck.BoardMain, 2)
	f.place(t, f.deckID, historyIsland, deck.BoardSideboard, 3)

	h := f.history(t)
	require.Len(t, h.Days, 1)
	wantDate := time.Now().In(mustParis(t)).Format("2006-01-02")
	assert.Equal(t, HistoryDay{Date: wantDate, Boards: []HistoryBoard{
		{Board: "main", Changes: []HistoryChange{{Name: "Lightning Bolt", Count: 2}}},
		{Board: "sideboard", Changes: []HistoryChange{{Name: "Island", Count: 3}}},
	}}, h.Days[0])
}

func mustParis(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)
	return loc
}

func TestPostgresRepository_GetDeckHistory_RemovingNetsOutTheSameDayAndLeavesNoLine(t *testing.T) {
	f := newHistoryFixture(t)
	cardID := f.collectionCopy(t, historyBolt, copyOptions{})
	f.collectionCopy(t, historyBolt, copyOptions{})
	f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 3)

	require.NoError(t, f.decks.RemoveCardFromDeck(context.Background(), f.userID, f.deckID, cardID))
	assert.Equal(t, map[string]int{"main Lightning Bolt": 2}, today(f.history(t)))

	f.place(t, f.deckID, historyIsland, deck.BoardMain, 1)
	var islandID int
	require.NoError(t, f.db.Get(&islandID, `SELECT id FROM tamiyo.deck_pending_cards WHERE deck_id = $1`, f.deckID))
	require.NoError(t, f.decks.RemovePendingCard(context.Background(), f.userID, f.deckID, islandID))
	assert.Equal(t, map[string]int{"main Lightning Bolt": 2}, today(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_MovingBetweenBoardsIsARemovalAndAnAddition(t *testing.T) {
	f := newHistoryFixture(t)
	id := f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 1)

	require.NoError(t, f.decks.PutCardInDeck(context.Background(), f.userID, f.deckID, id, deck.BoardSideboard))

	assert.Equal(t, map[string]int{"sideboard Lightning Bolt": 1}, today(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_PendingQuantityAndBoardChanges(t *testing.T) {
	f := newHistoryFixture(t)
	f.place(t, f.deckID, historyIsland, deck.BoardMain, 2)
	id := f.pendingID(t, "Island", deck.BoardMain)

	five, side := 5, deck.BoardSideboard
	_, err := f.decks.UpdatePendingCard(context.Background(), f.userID, f.deckID, id, &five, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"main Island": 5}, today(f.history(t)))

	_, err = f.decks.UpdatePendingCard(context.Background(), f.userID, f.deckID, id, nil, &side)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"sideboard Island": 5}, today(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_DeletingACollectionCardInADeckChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	id := f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardSideboard, 1)
	before := f.history(t)

	require.NoError(t, f.cards.DeleteCard(context.Background(), f.userID, id))

	assert.Equal(t, before, f.history(t))
	assert.Equal(t, 1, f.pendingQuantity(t, "Lightning Bolt"))
}

func TestPostgresRepository_GetDeckHistory_DeletingACollectionCardInTwoDecksChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	second := f.otherDeck(t)
	secondHistory := historyFixture{insightsFixture: second, decks: f.decks, cards: f.cards, service: f.service}
	id := f.collectionCopy(t, historyBolt, copyOptions{})
	require.NoError(t, f.decks.PutCardInDeck(context.Background(), f.userID, f.deckID, id, deck.BoardMain))
	require.NoError(t, f.decks.PutCardInDeck(context.Background(), f.userID, second.deckID, id, deck.BoardSideboard))
	before, secondBefore := f.history(t), secondHistory.history(t)

	require.NoError(t, f.cards.DeleteCard(context.Background(), f.userID, id))

	assert.Equal(t, before, f.history(t))
	assert.Equal(t, secondBefore, secondHistory.history(t))
}

func TestPostgresRepository_GetDeckHistory_DeletingACollectionCardMergedIntoAPendingRowChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 1)
	id := f.collectionCopy(t, historyBolt, copyOptions{})
	require.NoError(t, f.decks.PutCardInDeck(context.Background(), f.userID, f.deckID, id, deck.BoardMain))
	before := f.history(t)

	require.NoError(t, f.cards.DeleteCard(context.Background(), f.userID, id))

	assert.Equal(t, before, f.history(t))
	assert.Equal(t, 2, f.pendingQuantity(t, "Lightning Bolt"))
}

func (f historyFixture) pendingQuantity(t *testing.T, name string) int {
	t.Helper()
	var q int
	require.NoError(t, f.db.Get(&q, `SELECT quantity FROM tamiyo.deck_pending_cards WHERE deck_id = $1 AND name = $2`, f.deckID, name))
	return q
}

func TestPostgresRepository_GetDeckHistory_DeletingADeckLeavesNoHistory(t *testing.T) {
	f := newHistoryFixture(t)
	f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 2)

	require.NoError(t, f.decks.DeleteDeck(context.Background(), f.userID, f.deckID))

	var rows int
	require.NoError(t, f.db.Get(&rows, `SELECT count(*) FROM tamiyo.deck_history`))
	assert.Zero(t, rows)
}

func TestPostgresRepository_GetDeckHistory_DeletingTheUserSucceeds(t *testing.T) {
	f := newHistoryFixture(t)
	f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 1)

	_, err := f.db.Exec(`DELETE FROM tamiyo.users WHERE id = $1`, f.userID)

	require.NoError(t, err)
}

func TestPostgresRepository_GetDeckHistory_FollowsTheDecksReach(t *testing.T) {
	f := newHistoryFixture(t)
	_, err := f.db.Exec(`UPDATE tamiyo.deck SET visibility = 'private' WHERE id = $1`, f.deckID)
	require.NoError(t, err)
	bob := seedUser(t, f.db, "bob@example.com")

	_, err = f.service.GetDeckHistory(context.Background(), "", f.deckID, deck.ReachShared)
	assert.ErrorIs(t, err, deck.ErrNotFound)
	_, err = f.service.GetDeckHistory(context.Background(), bob, f.deckID, deck.ReachOwn)
	assert.ErrorIs(t, err, deck.ErrNotFound)

	_, err = f.db.Exec(`UPDATE tamiyo.deck SET visibility = 'public' WHERE id = $1`, f.deckID)
	require.NoError(t, err)
	_, err = f.service.GetDeckHistory(context.Background(), "", f.deckID, deck.ReachShared)
	assert.NoError(t, err)
}
