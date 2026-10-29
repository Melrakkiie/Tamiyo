//go:build integration

package deckinsights

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/bulk"
	"Melrakkiie/Tamiyo/internal/deck"
)

var historyIslandAlt = printing{ScryfallID: "33333333-3333-3333-3333-333333333333", Name: "Island", SetCode: "leb", CollectorNumber: "3", TypeLine: "Basic Land — Island"}

func (f historyFixture) ownedIDs(t *testing.T, deckID string) []int {
	t.Helper()
	var ids []int
	require.NoError(t, f.db.Select(&ids, `SELECT card_id FROM tamiyo.card_deck WHERE deck_id = $1 ORDER BY card_id`, deckID))
	return ids
}

func TestPostgresRepository_GetDeckHistory_CommittingAPendingCardChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 2)
	f.place(t, f.deckID, historyIsland, deck.BoardSideboard, 1)
	before := f.history(t)

	summary, err := f.decks.CommitPendingCards(context.Background(), f.userID, f.deckID, deck.PendingCommit{})

	require.NoError(t, err)
	require.Equal(t, 3, summary.CardsCreated)
	assert.Equal(t, before, f.history(t))
}

func TestPostgresRepository_GetDeckHistory_SwitchingFinishToAnotherCopyChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	f.collectionCopy(t, historyIsland, copyOptions{})
	f.place(t, f.deckID, historyIsland, deck.BoardMain, 1)
	f.collectionCopy(t, historyIsland, copyOptions{foil: true})
	before := today(f.history(t))

	summary, err := f.decks.SwitchFinish(context.Background(), f.userID, f.deckID, deck.CardSelection{CardIDs: f.ownedIDs(t, f.deckID)}, true)

	require.NoError(t, err)
	require.Equal(t, 1, summary.CardsLinked)
	assert.Equal(t, before, today(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_SwitchingFinishToAPendingCardChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	f.collectionCopy(t, historyIsland, copyOptions{})
	f.place(t, f.deckID, historyIsland, deck.BoardSideboard, 1)
	before := today(f.history(t))

	summary, err := f.decks.SwitchFinish(context.Background(), f.userID, f.deckID, deck.CardSelection{CardIDs: f.ownedIDs(t, f.deckID)}, true)

	require.NoError(t, err)
	require.Equal(t, 1, summary.CardsPending)
	assert.Equal(t, before, today(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_SwitchingProxyChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 1)
	f.collectionCopy(t, historyBolt, copyOptions{proxy: true})
	f.place(t, f.deckID, historyIsland, deck.BoardMain, 1)
	before := today(f.history(t))

	// The Bolt finds a proxy copy, the pending Island falls back to a pending proxy.
	selection := deck.CardSelection{CardIDs: f.ownedIDs(t, f.deckID), PendingIDs: []int{f.pendingID(t, historyIsland.ScryfallID, deck.BoardMain)}}
	summary, err := f.decks.SwitchProxy(context.Background(), f.userID, f.deckID, selection, true)

	require.NoError(t, err)
	require.Equal(t, 2, summary.CardsLinked+summary.CardsPending)
	assert.Equal(t, before, today(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_SplittingPrintingsChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	seedPrintings(t, f.db, historyIslandAlt)
	f.collectionCopy(t, historyIsland, copyOptions{})
	f.collectionCopy(t, historyIsland, copyOptions{})
	f.place(t, f.deckID, historyIsland, deck.BoardSideboard, 2)
	before := today(f.history(t))
	alt := deck.PrintingChoice{ScryfallID: historyIslandAlt.ScryfallID, SetCode: historyIslandAlt.SetCode, CollectorNumber: historyIslandAlt.CollectorNumber}

	summary, err := f.decks.SplitPrintings(context.Background(), f.userID, f.deckID, deck.CardSelection{CardIDs: f.ownedIDs(t, f.deckID)}, []deck.PrintingChoice{alt, alt})

	require.NoError(t, err)
	require.Equal(t, 2, summary.CardsRemoved)
	assert.Equal(t, before, today(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_SettingACommanderAmongTheMainBoardChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	id := f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 1)
	before := f.history(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", f.userID) })
	deck.NewHandler(f.decks).RegisterRoutes(router)

	body := `{"commander_id": ` + strconv.Itoa(id) + `}`
	req := httptest.NewRequest(http.MethodPatch, "/deck/"+f.deckID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, before, f.history(t))
}

func TestPostgresRepository_GetDeckHistory_AddingACommanderNotInTheDeckIsAnAdditionToTheMainBoard(t *testing.T) {
	f := newHistoryFixture(t)
	f.place(t, f.deckID, historyIsland, deck.BoardMain, 1)
	f.collectionCopy(t, historyBolt, copyOptions{})
	line := deck.PlacementLine{
		Printing: deck.Printing{Name: historyBolt.Name, ScryfallID: historyBolt.ScryfallID, SetCode: historyBolt.SetCode, CollectorNumber: historyBolt.CollectorNumber},
		Board:    deck.BoardMain, Quantity: 1, Commander: true,
	}

	_, err := f.decks.PlaceCards(context.Background(), f.userID, f.deckID, deck.PlacementPlan{Lines: []deck.PlacementLine{line}})

	require.NoError(t, err)
	assert.Equal(t, map[string]int{"main Island": 1, "main Lightning Bolt": 1}, today(f.history(t)))
}

func TestPostgresRepository_GetDeckHistory_CollectingTheOwnDeckChangesNothing(t *testing.T) {
	f := newHistoryFixture(t)
	f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 2)
	f.place(t, f.deckID, historyIsland, deck.BoardSideboard, 1)
	before := f.history(t)

	_, err := f.decks.CollectOwnDeck(context.Background(), f.userID, f.deckID, deck.OwnDeckCollect{Boards: []string{deck.BoardMain, deck.BoardSideboard}, OwnedCopies: true})

	require.NoError(t, err)
	assert.Equal(t, before, f.history(t))
}

func TestPostgresRepository_GetDeckHistory_CollectingASharedDeckLeavesItsHistoryAlone(t *testing.T) {
	f := newHistoryFixture(t)
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 2)
	_, err := f.db.Exec(`UPDATE tamiyo.deck SET visibility = 'public' WHERE id = $1`, f.deckID)
	require.NoError(t, err)
	bob := seedUser(t, f.db, "bob@example.com")
	before := f.history(t)

	_, err = f.decks.CollectSharedDeck(context.Background(), bob, f.deckID, deck.SharedDeckCollect{Boards: []string{deck.BoardMain}})

	require.NoError(t, err)
	assert.Equal(t, before, f.history(t))
}

func TestPostgresRepository_GetDeckHistory_PlacingACopyUsedByAnotherDeckIsAnAdditionInTheReceivingDeck(t *testing.T) {
	f := newHistoryFixture(t)
	source := f
	source.insightsFixture = f.otherDeck(t)
	f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, source.deckID, historyBolt, deck.BoardMain, 1)
	sourceBefore := source.history(t)

	f.place(t, f.deckID, historyBolt, deck.BoardSideboard, 1)

	assert.Equal(t, map[string]int{"sideboard Lightning Bolt": 1}, today(f.history(t)))
	assert.Equal(t, sourceBefore, source.history(t))
	assert.Equal(t, 1, len(f.ownedIDs(t, source.deckID)), "the copy stays in the source deck")
}

func TestPostgresRepository_GetDeckHistory_DuplicatingADeckStartsTheCopyWithItsCardsAsAdditions(t *testing.T) {
	f := newHistoryFixture(t)
	f.collectionCopy(t, historyBolt, copyOptions{})
	f.place(t, f.deckID, historyBolt, deck.BoardMain, 2)
	f.place(t, f.deckID, historyIsland, deck.BoardSideboard, 3)
	// The source's own history has a card that left again: it must not reach the copy.
	f.place(t, f.deckID, historyIsland, deck.BoardMain, 1)
	require.NoError(t, f.decks.RemovePendingCard(context.Background(), f.userID, f.deckID, f.pendingID(t, historyIsland.ScryfallID, deck.BoardMain)))
	sourceBefore := f.history(t)

	result, err := f.decks.DuplicateDeck(context.Background(), f.userID, f.deckID, deck.DuplicateRequest{})
	require.NoError(t, err)

	copied := f
	copied.deckID = result.Deck.ID
	assert.Equal(t, map[string]int{"main Lightning Bolt": 2, "sideboard Island": 3}, today(copied.history(t)))
	assert.Equal(t, sourceBefore, f.history(t))
}

func (f historyFixture) pendingID(t *testing.T, scryfallID, board string) int {
	t.Helper()
	var id int
	require.NoError(t, f.db.Get(&id, `SELECT id FROM tamiyo.deck_pending_cards WHERE deck_id = $1 AND scryfall_id = $2 AND board = $3`, f.deckID, scryfallID, board))
	return id
}

// listResolver answers every Moxfield-style "(SET) number" identifier with the fixture printings.
type listResolver struct{ byKey map[string]bulk.ResolvedCard }

func (r listResolver) SetNames(context.Context) (map[string]string, error) { return nil, nil }

func (r listResolver) Resolve(_ context.Context, ids []bulk.CardIdentifier) (map[string]bulk.ResolvedCard, error) {
	out := map[string]bulk.ResolvedCard{}
	for _, id := range ids {
		key := "sc:" + strings.ToLower(id.SetCode) + "#" + id.CollectorNumber
		if rc, ok := r.byKey[key]; ok {
			out[key] = rc
		}
	}
	return out, nil
}

func TestPostgresRepository_GetDeckHistory_ImportingAListGivesItsCardsAsAdditions(t *testing.T) {
	f := newHistoryFixture(t)
	resolver := listResolver{byKey: map[string]bulk.ResolvedCard{
		"sc:lea#1": {ScryfallID: historyIsland.ScryfallID, Name: "Island", SetCode: "lea", CollectorNumber: "1"},
		"sc:lea#2": {ScryfallID: historyBolt.ScryfallID, Name: "Lightning Bolt", SetCode: "lea", CollectorNumber: "2"},
	}}
	importer := bulk.NewService(nil, nil, f.decks, resolver)

	_, err := importer.ImportIntoDeck(context.Background(), f.userID, f.deckID, bulk.DeckImportOptions{}, strings.NewReader("3 Island (LEA) 1\n1 Lightning Bolt (LEA) 2\n"))

	require.NoError(t, err)
	assert.Equal(t, map[string]int{"main Island": 3, "main Lightning Bolt": 1}, today(f.history(t)))
}
