package bulk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

const (
	islandONE = "aaaaaaaa-0000-0000-0000-0000000000a1"
	islandBRO = "aaaaaaaa-0000-0000-0000-0000000000a2"
)

var (
	neoIsland = PrintingChoice{ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294"}
	oneIsland = PrintingChoice{ScryfallID: islandONE, SetCode: "one", CollectorNumber: "263"}
	broIsland = PrintingChoice{ScryfallID: islandBRO, SetCode: "bro", CollectorNumber: "278", Foil: true}
)

func printingsFixture() (*fakeCardService, *fakeDeckService) {
	cards, decks := foilFixture()
	cards.allCards = append(cards.allCards, card.Card{ID: 50, Name: "Island", ScryfallID: islandONE, SetCode: "one", CollectorNumber: "263"})
	return cards, decks
}

func repeat(p PrintingChoice, n int) []PrintingChoice {
	out := make([]PrintingChoice, n)
	for i := range out {
		out[i] = p
	}
	return out
}

func TestSplitPrintings_UnchangedSplitIsANoOp(t *testing.T) {
	cards, decks := printingsFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SplitPrintings(context.Background(), testUserID, fixtureDeckID, PrintingSplit{
		CardIDs: []int{2, 3}, PendingIDs: []int{8}, Printings: repeat(neoIsland, 5),
	})

	require.NoError(t, err)
	assert.Zero(t, summary)
	assert.Empty(t, decks.unlinkedCards)
	assert.Empty(t, decks.removedPending)
	assert.Empty(t, decks.addedPending)
}

func TestSplitPrintings_OwnedCopyTakesAnOwnedCopyOfTheOtherPrinting(t *testing.T) {
	cards, decks := printingsFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SplitPrintings(context.Background(), testUserID, fixtureDeckID, PrintingSplit{
		CardIDs: []int{2, 3}, Printings: []PrintingChoice{oneIsland, neoIsland},
	})

	require.NoError(t, err)
	assert.Equal(t, []int{3}, decks.unlinkedCards, "the first copy keeps its printing, the second leaves the deck")
	assert.Equal(t, []int{50}, decks.linkedCards[fixtureDeckID])
	assert.Equal(t, deck.BoardMain, decks.linkedBoards[50])
	assert.Empty(t, decks.addedPending)
	assert.Empty(t, cards.created)
	assert.Equal(t, Summary{CardsLinked: 1, CardsRemoved: 1}, summary)
}

func TestSplitPrintings_GoesPendingWithoutACopyOfThatPrinting(t *testing.T) {
	cards, decks := printingsFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SplitPrintings(context.Background(), testUserID, fixtureDeckID, PrintingSplit{
		CardIDs: []int{2, 3}, Printings: []PrintingChoice{neoIsland, broIsland},
	})

	require.NoError(t, err)
	assert.Equal(t, []int{3}, decks.unlinkedCards)
	require.Len(t, decks.addedPending, 1)
	pending := decks.addedPending[0]
	assert.Equal(t, "Island", pending.Name)
	assert.Equal(t, islandBRO, pending.ScryfallID)
	assert.Equal(t, "bro", pending.SetCode)
	assert.Equal(t, "278", pending.CollectorNumber)
	assert.True(t, pending.Foil)
	assert.Equal(t, 1, pending.Quantity)
	assert.Equal(t, deck.BoardMain, pending.Board)
	assert.Empty(t, cards.created)
	assert.Equal(t, Summary{CardsPending: 1, CardsRemoved: 1}, summary)
}

func TestSplitPrintings_SplitsAPendingCard(t *testing.T) {
	cards, decks := printingsFixture()
	decks.pending[1].Quantity = 10
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SplitPrintings(context.Background(), testUserID, fixtureDeckID, PrintingSplit{
		PendingIDs: []int{8}, Printings: append(repeat(neoIsland, 7), repeat(oneIsland, 3)...),
	})

	require.NoError(t, err)
	assert.Equal(t, map[int]int{8: 7}, decks.pendingQuantities)
	assert.Empty(t, decks.removedPending)
	assert.Equal(t, []int{50}, decks.linkedCards[fixtureDeckID], "one of the three ONE Islands is owned")
	require.Len(t, decks.addedPending, 1)
	assert.Equal(t, islandONE, decks.addedPending[0].ScryfallID)
	assert.Equal(t, 2, decks.addedPending[0].Quantity)
	assert.False(t, decks.addedPending[0].Foil)
	assert.Equal(t, Summary{CardsLinked: 1, CardsPending: 2, CardsRemoved: 3}, summary)

	decks = tamiyoDeckFixture()
	svc = NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})
	summary, err = svc.SplitPrintings(context.Background(), testUserID, fixtureDeckID, PrintingSplit{
		PendingIDs: []int{8}, Printings: repeat(broIsland, 3),
	})
	require.NoError(t, err)
	assert.Equal(t, []int{8}, decks.removedPending)
	assert.Equal(t, Summary{CardsPending: 3, CardsRemoved: 3}, summary)
}

func TestSplitPrintings_RejectsMismatches(t *testing.T) {
	cards, decks := printingsFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})
	split := func(req PrintingSplit) error {
		_, err := svc.SplitPrintings(context.Background(), testUserID, fixtureDeckID, req)
		return err
	}

	assert.ErrorIs(t, split(PrintingSplit{CardIDs: []int{2, 3}, Printings: repeat(oneIsland, 3)}), ErrSplitMismatch)
	assert.ErrorIs(t, split(PrintingSplit{CardIDs: []int{1, 2}, Printings: repeat(oneIsland, 2)}), ErrSplitMismatch)
	assert.ErrorIs(t, split(PrintingSplit{CardIDs: []int{2}, PendingIDs: []int{9}, Printings: repeat(oneIsland, 2)}), ErrSplitMismatch)
	assert.ErrorIs(t, split(PrintingSplit{CardIDs: []int{2, 21}, Printings: repeat(oneIsland, 2)}), ErrCardsNotInDeck)
	assert.Empty(t, decks.unlinkedCards)
	assert.Empty(t, decks.addedPending)

	_, err := svc.SplitPrintings(context.Background(), testUserID, "00000000-0000-0000-0000-000000000099", PrintingSplit{CardIDs: []int{2}, Printings: repeat(oneIsland, 1)})
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestSplitPrintings_RejectsTheCommander(t *testing.T) {
	cards, decks := printingsFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.SplitPrintings(context.Background(), testUserID, fixtureDeckID, PrintingSplit{PendingIDs: []int{7}, Printings: repeat(oneIsland, 1)})
	assert.ErrorIs(t, err, ErrSplitMismatch)

	decks.decks[0].CommanderPendingID = nil
	decks.decks[0].CommanderID = ptr(2)
	_, err = svc.SplitPrintings(context.Background(), testUserID, fixtureDeckID, PrintingSplit{CardIDs: []int{2, 3}, Printings: repeat(oneIsland, 2)})
	assert.ErrorIs(t, err, ErrSplitMismatch)
	assert.Empty(t, decks.unlinkedCards)
}

func TestHandler_SplitPrintings(t *testing.T) {
	service := &fakeImportService{summary: Summary{CardsLinked: 1, CardsRemoved: 1}}
	router := setupRouter(service)

	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/deck/"+fixtureDeckID+"/split-printings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		return w
	}

	w := post(`{"card_ids": [2], "pending_ids": [8], "printings": [
		{"scryfall_id": "` + islandONE + `", "set_code": "one", "collector_number": "263", "foil": true},
		{"scryfall_id": "` + islandNEO + `", "set_code": "neo", "collector_number": "294"}]}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, PrintingSplit{CardIDs: []int{2}, PendingIDs: []int{8}, Printings: []PrintingChoice{
		{ScryfallID: islandONE, SetCode: "one", CollectorNumber: "263", Foil: true}, neoIsland,
	}}, service.lastSplit)
	assert.Contains(t, w.Body.String(), `"cards_linked":1`)

	assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2], "printings": []}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(`{"printings": [{"scryfall_id": "x"}]}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2], "printings": [{"set_code": "one"}]}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(`nope`).Code)

	service.err = ErrSplitMismatch
	assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2], "printings": [{"scryfall_id": "x"}]}`).Code)
	service.err = ErrDeckNotFound
	assert.Equal(t, http.StatusNotFound, post(`{"card_ids": [2], "printings": [{"scryfall_id": "x"}]}`).Code)
}
