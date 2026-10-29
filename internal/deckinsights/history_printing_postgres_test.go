//go:build integration

package deckinsights

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
)

// annotated is a history line's name, count and remembered printing.
type annotated struct {
	Count  int
	Set    string
	Number string
	Foil   bool
}

// lines flattens the history to "board name" -> annotated line; a line
// without annotation has an empty Set.
func lines(h DeckHistory) map[string]annotated {
	out := map[string]annotated{}
	for _, d := range h.Days {
		for _, b := range d.Boards {
			for _, c := range b.Changes {
				a := annotated{Count: c.Count}
				if c.SetCode != nil {
					a.Set, a.Number, a.Foil = *c.SetCode, *c.CollectorNumber, *c.Foil
				}
				out[b.Board+" "+c.Name] = a
			}
		}
	}
	return out
}

// forgetHistory starts a clean day: what was set up before no longer counts.
func (f historyFixture) forgetHistory(t *testing.T) {
	t.Helper()
	_, err := f.db.Exec(`DELETE FROM tamiyo.deck_history WHERE deck_id = $1`, f.deckID)
	require.NoError(t, err)
}

func TestPostgresRepository_GetDeckHistory_AdditionsAndRemovalsRememberTheirCopy(t *testing.T) {
	f := newHistoryFixture(t)
	f.ownedCopy(t, historyBolt, copyOptions{foil: true})
	f.pendingCard(t, historyIsland, 2, copyOptions{board: deck.BoardSideboard})

	assert.Equal(t, map[string]annotated{
		"main Lightning Bolt": {Count: 1, Set: "lea", Number: "2", Foil: true},
		"sideboard Island":    {Count: 2, Set: "lea", Number: "1"},
	}, lines(f.history(t)))

	f.forgetHistory(t)
	require.NoError(t, f.decks.RemoveCardFromDeck(context.Background(), f.userID, f.deckID, f.ownedIDs(t, f.deckID)[0]))
	require.NoError(t, f.decks.RemovePendingCard(context.Background(), f.userID, f.deckID, f.pendingID(t, historyIsland.ScryfallID, deck.BoardSideboard)))

	assert.Equal(t, map[string]annotated{
		"main Lightning Bolt": {Count: -1, Set: "lea", Number: "2", Foil: true},
		"sideboard Island":    {Count: -2, Set: "lea", Number: "1"},
	}, lines(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_AMixedDayKeepsTheCopyOfTheLinesSign(t *testing.T) {
	f := newHistoryFixture(t)
	plain := f.collectionCopy(t, historyBolt, copyOptions{})
	_, err := f.db.Exec(`INSERT INTO tamiyo.card_deck (card_id, deck_id, board) VALUES ($1, $2, 'main')`, plain, f.deckID)
	require.NoError(t, err)
	f.forgetHistory(t)

	f.ownedCopy(t, historyBolt, copyOptions{foil: true})
	f.ownedCopy(t, historyBolt, copyOptions{foil: true})
	require.NoError(t, f.decks.RemoveCardFromDeck(context.Background(), f.userID, f.deckID, plain))

	assert.Equal(t, annotated{Count: 1, Set: "lea", Number: "2", Foil: true}, lines(f.history(t))["main Lightning Bolt"])
}

func TestPostgresRepository_GetDeckHistory_AMixedDayRemovalKeepsTheRemovedCopy(t *testing.T) {
	f := newHistoryFixture(t)
	f.ownedCopy(t, historyBolt, copyOptions{})
	f.ownedCopy(t, historyBolt, copyOptions{})
	f.forgetHistory(t)

	f.ownedCopy(t, historyBolt, copyOptions{foil: true})
	for _, id := range f.ownedIDs(t, f.deckID)[:2] {
		require.NoError(t, f.decks.RemoveCardFromDeck(context.Background(), f.userID, f.deckID, id))
	}

	assert.Equal(t, annotated{Count: -1, Set: "lea", Number: "2"}, lines(f.history(t))["main Lightning Bolt"])
}

func TestPostgresRepository_GetDeckHistory_ASignFlipTakesThePrintingOfTheFlippingMovement(t *testing.T) {
	f := newHistoryFixture(t)
	f.pendingCard(t, historyIsland, 3, copyOptions{})
	f.forgetHistory(t)

	f.pendingCard(t, historyIslandAlt, 1, copyOptions{})
	require.NoError(t, f.decks.RemovePendingCard(context.Background(), f.userID, f.deckID, f.pendingID(t, historyIsland.ScryfallID, deck.BoardMain)))

	assert.Equal(t, annotated{Count: -2, Set: "lea", Number: "1"}, lines(f.history(t))["main Island"])
}

func TestPostgresRepository_GetDeckHistory_MovingBetweenBoardsAnnotatesBothLines(t *testing.T) {
	f := newHistoryFixture(t)
	id := f.collectionCopy(t, historyBolt, copyOptions{foil: true})
	_, err := f.db.Exec(`INSERT INTO tamiyo.card_deck (card_id, deck_id, board) VALUES ($1, $2, 'main')`, id, f.deckID)
	require.NoError(t, err)
	f.forgetHistory(t)

	require.NoError(t, f.decks.PutCardInDeck(context.Background(), f.userID, f.deckID, id, deck.BoardSideboard))

	assert.Equal(t, map[string]annotated{
		"main Lightning Bolt":      {Count: -1, Set: "lea", Number: "2", Foil: true},
		"sideboard Lightning Bolt": {Count: 1, Set: "lea", Number: "2", Foil: true},
	}, lines(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_ARowWrittenBeforeTheAnnotationReadsBackWithoutIt(t *testing.T) {
	f := newHistoryFixture(t)
	_, err := f.db.Exec(`INSERT INTO tamiyo.deck_history (deck_id, day, board, name, count) VALUES ($1, '2026-01-02', 'main', 'Sol Ring', 2)`, f.deckID)
	require.NoError(t, err)

	h := f.history(t)

	assert.Equal(t, []HistoryChange{{Name: "Sol Ring", Count: 2}}, h.Days[0].Boards[0].Changes)
}
