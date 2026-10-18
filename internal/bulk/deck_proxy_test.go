package bulk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

func proxyFixture() (*fakeCardService, *fakeDeckService) {
	cards, decks := foilFixture()
	cards.allCards = append(cards.allCards,
		card.Card{ID: 50, Name: "Island", ScryfallID: islandNEO, Proxy: true},
		card.Card{ID: 51, Name: "Island", ScryfallID: islandNEO, Foil: true, Proxy: true},
	)
	return cards, decks
}

func TestSwitchProxy_LinksAFreeProxyCopyOfTheSameFinish(t *testing.T) {
	cards, decks := proxyFixture()
	svc := newTestService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchProxy(context.Background(), testUserID, fixtureDeckID, ProxySwitch{CardIDs: []int{2}, Proxy: true})

	require.NoError(t, err)
	assert.Equal(t, []int{50}, decks.linkedCards[fixtureDeckID], "the non-foil proxy, not the foil one")
	assert.Equal(t, deck.BoardMain, decks.linkedBoards[50])
	assert.Equal(t, []int{2}, decks.unlinkedCards, "the real copy leaves the deck but stays in the collection")
	assert.Empty(t, decks.addedPending)
	assert.Empty(t, cards.created)
	assert.Equal(t, Summary{CardsLinked: 1, CardsRemoved: 1}, summary)
}

func TestSwitchProxy_GoesPendingWithoutAFreeProxyCopy(t *testing.T) {
	cards, decks := proxyFixture()
	svc := newTestService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchProxy(context.Background(), testUserID, fixtureDeckID, ProxySwitch{CardIDs: []int{4}, Proxy: true})

	require.NoError(t, err)
	require.Len(t, decks.addedPending, 1)
	pending := decks.addedPending[0]
	assert.Equal(t, "Duress", pending.Name)
	assert.True(t, pending.Proxy)
	assert.False(t, pending.Foil)
	assert.Equal(t, 1, pending.Quantity)
	assert.Equal(t, deck.BoardSideboard, pending.Board)
	assert.Equal(t, []int{4}, decks.unlinkedCards)
	assert.Empty(t, cards.created)
	assert.Equal(t, Summary{CardsPending: 1, CardsRemoved: 1}, summary)
}

func TestSwitchProxy_PendingProxySwitchesBack(t *testing.T) {
	cards, decks := proxyFixture()
	decks.pending = append(decks.pending, deck.PendingCard{ID: 10, Name: "Island", ScryfallID: islandNEO, SetCode: "neo", CollectorNumber: "294", Quantity: 3, Proxy: true, Board: deck.BoardMain})
	cards.allCards = append(cards.allCards,
		card.Card{ID: 70, Name: "Island", ScryfallID: islandNEO},
		card.Card{ID: 71, Name: "Island", ScryfallID: islandNEO},
	)
	svc := newTestService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchProxy(context.Background(), testUserID, fixtureDeckID, ProxySwitch{PendingIDs: []int{10, 8}, Proxy: false})

	require.NoError(t, err)
	assert.Equal(t, []int{70, 71}, decks.linkedCards[fixtureDeckID], "the free real non-foil Islands, never the proxies")
	require.Len(t, decks.addedPending, 1)
	assert.Equal(t, 1, decks.addedPending[0].Quantity)
	assert.False(t, decks.addedPending[0].Proxy)
	assert.Equal(t, []int{10}, decks.removedPending, "the pending Islands that are not proxies stay as they are")
	assert.Equal(t, Summary{CardsLinked: 2, CardsPending: 1, CardsRemoved: 3}, summary)
}

func TestSwitchProxy_MovesTheCommander(t *testing.T) {
	cards, decks := proxyFixture()
	decks.decks[0].CommanderPendingID = nil
	decks.decks[0].CommanderID = ptr(1)
	svc := newTestService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.SwitchProxy(context.Background(), testUserID, fixtureDeckID, ProxySwitch{CardIDs: []int{1}, Proxy: true})
	require.NoError(t, err)
	require.Len(t, decks.addedPending, 1)
	assert.True(t, decks.addedPending[0].Proxy)
	assert.Equal(t, decks.addedPending[0].ID, decks.pendingCommanderID)

	cards.allCards = append(cards.allCards, card.Card{ID: 60, Name: "Sol Ring", ScryfallID: solRingSLD, Proxy: true})
	_, err = svc.SwitchProxy(context.Background(), testUserID, fixtureDeckID, ProxySwitch{CardIDs: []int{1}, Proxy: true})
	require.NoError(t, err)
	assert.Equal(t, 60, decks.cardCommanderID)
}

func TestSwitchProxy_IgnoresCardsAlreadyInThatStateAndRejectsOthers(t *testing.T) {
	cards, decks := proxyFixture()
	svc := newTestService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.SwitchProxy(context.Background(), testUserID, fixtureDeckID, ProxySwitch{CardIDs: []int{1}, Proxy: false})
	require.NoError(t, err)
	assert.Zero(t, summary)

	_, err = svc.SwitchProxy(context.Background(), testUserID, fixtureDeckID, ProxySwitch{CardIDs: []int{1, 21}, Proxy: true})
	assert.ErrorIs(t, err, ErrCardsNotInDeck)
	_, err = svc.SwitchProxy(context.Background(), testUserID, fixtureDeckID, ProxySwitch{PendingIDs: []int{99}, Proxy: true})
	assert.ErrorIs(t, err, ErrCardsNotInDeck)
	assert.Empty(t, decks.unlinkedCards)
	assert.Empty(t, decks.removedPending)
}
