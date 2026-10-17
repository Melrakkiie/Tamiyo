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

const otherDeckID = "00000000-0000-0000-0000-000000000002"

func foilFixture() (*fakeCardService, *fakeDeckService) {
	decks := tamiyoDeckFixture()
	decks.decks = append(decks.decks, deck.Deck{ID: otherDeckID, Name: "Autre"})
	decks.cardsByDeck[otherDeckID] = []deck.DeckCard{{ID: 21, Name: "Island", ScryfallID: islandNEO, Foil: true}}
	cards := &fakeCardService{allCards: []card.Card{
		{ID: 1, Name: "Sol Ring", ScryfallID: solRingSLD},
		{ID: 2, Name: "Island", ScryfallID: islandNEO},
		{ID: 3, Name: "Island", ScryfallID: islandNEO},
		{ID: 4, Name: "Duress", ScryfallID: duressM19},
		{ID: 20, Name: "Island", ScryfallID: islandNEO, Foil: true},
		{ID: 21, Name: "Island", ScryfallID: islandNEO, Foil: true},
	}}
	return cards, decks
}

func TestSwitchFoil_UsesOwnedCopiesOfTheOtherFinishFreeOnesFirst(t *testing.T) {
	cards, decks := foilFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{CardIDs: []int{2, 3}, Foil: true})

	require.NoError(t, err)
	assert.Equal(t, []int{20, 21}, decks.linkedCards[fixtureDeckID], "the copy in no deck first, then the one another deck uses")
	assert.Equal(t, deck.BoardMain, decks.linkedBoards[20])
	assert.Equal(t, []int{2, 3}, decks.unlinkedCards, "the non-foil copies leave the deck but stay in the collection")
	assert.Empty(t, decks.addedPending)
	assert.Empty(t, cards.created)
	assert.Equal(t, Summary{CardsLinked: 2, CardsRemoved: 2}, summary)
}

func TestSwitchFoil_GoesPendingWithoutACopyOfThatFinish(t *testing.T) {
	cards, decks := foilFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{CardIDs: []int{4}, Foil: true})

	require.NoError(t, err)
	require.Len(t, decks.addedPending, 1)
	pending := decks.addedPending[0]
	assert.Equal(t, "Duress", pending.Name)
	assert.True(t, pending.Foil)
	assert.Equal(t, 1, pending.Quantity)
	assert.Equal(t, deck.BoardSideboard, pending.Board)
	assert.Equal(t, duressM19, pending.ScryfallID)
	assert.Equal(t, []int{4}, decks.unlinkedCards)
	assert.Empty(t, decks.linkedCards[fixtureDeckID])
	assert.Equal(t, Summary{CardsPending: 1, CardsRemoved: 1}, summary)
}

func TestSwitchFoil_LeavesProxyCopiesForNonProxyCards(t *testing.T) {
	cards, decks := foilFixture()
	cards.allCards = append(cards.allCards, card.Card{ID: 30, Name: "Duress", ScryfallID: duressM19, Foil: true, Proxy: true})
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{CardIDs: []int{4}, Foil: true})

	require.NoError(t, err)
	assert.Empty(t, decks.linkedCards[fixtureDeckID], "the foil Duress is a proxy, the deck card is not")
	require.Len(t, decks.addedPending, 1)
	assert.True(t, decks.addedPending[0].Foil)
	assert.False(t, decks.addedPending[0].Proxy)
	assert.Equal(t, Summary{CardsPending: 1, CardsRemoved: 1}, summary)
}

func TestSwitchFoil_KeepsTheCommander(t *testing.T) {
	cards, decks := foilFixture()
	decks.decks[0].CommanderPendingID = nil
	decks.decks[0].CommanderID = ptr(1)
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{CardIDs: []int{1}, Foil: true})
	require.NoError(t, err)
	require.Len(t, decks.addedPending, 1)
	assert.Equal(t, decks.addedPending[0].ID, decks.pendingCommanderID)

	cards.allCards = append(cards.allCards, card.Card{ID: 30, Name: "Sol Ring", ScryfallID: solRingSLD, Foil: true})
	_, err = svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{CardIDs: []int{1}, Foil: true})
	require.NoError(t, err)
	assert.Equal(t, 30, decks.cardCommanderID)
}

func TestSwitchFoil_IgnoresCardsAlreadyInThatFinishAndRejectsOthers(t *testing.T) {
	cards, decks := foilFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{CardIDs: []int{1, 1}, Foil: false})
	require.NoError(t, err)
	assert.Zero(t, summary)
	assert.Empty(t, decks.unlinkedCards)

	_, err = svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{CardIDs: []int{1, 21}, Foil: true})
	assert.ErrorIs(t, err, ErrCardsNotInDeck)
	assert.Empty(t, decks.unlinkedCards)

	_, err = svc.SwitchFoil(context.Background(), testUserID, "00000000-0000-0000-0000-000000000099", FoilSwitch{CardIDs: []int{1}, Foil: true})
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestSwitchFoil_PendingCardsTakeOwnedCopiesOfTheNewFinish(t *testing.T) {
	cards, decks := foilFixture()
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{PendingIDs: []int{8}, Foil: true})

	require.NoError(t, err)
	assert.Equal(t, []int{20, 21}, decks.linkedCards[fixtureDeckID], "two of the three pending Islands have a foil copy")
	assert.Equal(t, deck.BoardMain, decks.linkedBoards[21])
	require.Len(t, decks.addedPending, 1)
	assert.Equal(t, 1, decks.addedPending[0].Quantity)
	assert.True(t, decks.addedPending[0].Foil)
	assert.Equal(t, deck.BoardMain, decks.addedPending[0].Board)
	assert.Equal(t, []int{8}, decks.removedPending)
	assert.Equal(t, Summary{CardsLinked: 2, CardsPending: 1, CardsRemoved: 3}, summary)
}

func TestSwitchFoil_PendingCommanderBecomesOwned(t *testing.T) {
	cards, decks := foilFixture()
	cards.allCards = append(cards.allCards, card.Card{ID: 40, Name: "Tamiyo, Inquisitive Student", ScryfallID: tamiyoNEO, Foil: true})
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{PendingIDs: []int{7}, Foil: true})
	require.NoError(t, err)
	assert.Equal(t, 40, decks.cardCommanderID)
	assert.Empty(t, decks.addedPending)
	assert.Equal(t, []int{7}, decks.removedPending)

	summary, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{PendingIDs: []int{9}, Foil: false})
	require.NoError(t, err)
	assert.Equal(t, Summary{CardsPending: 1, CardsRemoved: 1}, summary, "no non-foil Duress free: the considering Duress stays pending")
	assert.Equal(t, deck.BoardConsidering, decks.addedPending[0].Board)
	assert.False(t, decks.addedPending[0].Foil)

	_, err = svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{PendingIDs: []int{99}, Foil: true})
	assert.ErrorIs(t, err, ErrCardsNotInDeck)
}

func TestHandler_SwitchDeckCards(t *testing.T) {
	cases := []struct {
		field string
		last  func(*fakeImportService) any
		want  func(cardIDs, pendingIDs []int, value bool) any
	}{
		{
			field: "foil",
			last:  func(s *fakeImportService) any { return s.lastFoilSwitch },
			want: func(cardIDs, pendingIDs []int, value bool) any {
				return FoilSwitch{CardIDs: cardIDs, PendingIDs: pendingIDs, Foil: value}
			},
		},
		{
			field: "proxy",
			last:  func(s *fakeImportService) any { return s.lastProxySwitch },
			want: func(cardIDs, pendingIDs []int, value bool) any {
				return ProxySwitch{CardIDs: cardIDs, PendingIDs: pendingIDs, Proxy: value}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			service := &fakeImportService{summary: Summary{CardsLinked: 1, CardsRemoved: 1}}
			router := setupRouter(service)

			post := func(body string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				body = strings.ReplaceAll(body, "FIELD", tc.field)
				req := httptest.NewRequest(http.MethodPost, "/deck/"+fixtureDeckID+"/switch-"+tc.field, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, req)
				return w
			}

			w := post(`{"card_ids": [2, 3], "FIELD": true}`)
			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tc.want([]int{2, 3}, nil, true), tc.last(service))
			assert.Contains(t, w.Body.String(), `"cards_linked":1`)

			assert.Equal(t, http.StatusOK, post(`{"pending_ids": [8], "FIELD": false}`).Code)
			assert.Equal(t, tc.want(nil, []int{8}, false), tc.last(service))
			assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2]}`).Code)
			assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [], "FIELD": false}`).Code)
			assert.Equal(t, http.StatusBadRequest, post(`nope`).Code)

			service.err = ErrCardsNotInDeck
			assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [9], "FIELD": true}`).Code)
			service.err = ErrDeckNotFound
			assert.Equal(t, http.StatusNotFound, post(`{"card_ids": [9], "FIELD": true}`).Code)
		})
	}
}
