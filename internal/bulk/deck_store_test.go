package bulk

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

func storeFixture() (*fakeCardService, *fakeDeckService, *fakeStorageService) {
	decks := tamiyoDeckFixture()
	decks.cardsByDeck[fixtureDeckID][1].StorageID = ptr(4)
	decks.cardsByDeck[fixtureDeckID][2].StorageID = ptr(5)
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 4, Name: "Box", Type: "box"}, {ID: 5, Name: "Binder", Type: "binder"}}}
	return &fakeCardService{}, decks, storages
}

func TestStoreDeck_MovesOwnedCopiesOfTheChosenBoards(t *testing.T) {
	cards, decks, storages := storeFixture()
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.StoreDeck(context.Background(), testUserID, fixtureDeckID, 4, []string{deck.BoardMain})

	require.NoError(t, err)
	assert.Equal(t, StoreSummary{CardsMoved: 2}, summary)
	assert.Equal(t, []int{1, 3}, cards.movedIDs)
	assert.Equal(t, 4, cards.movedStorage)
}

func TestStoreDeck_IncludesOtherBoardsWhenAsked(t *testing.T) {
	cards, decks, storages := storeFixture()
	svc := NewService(cards, storages, decks, &fakeResolver{})

	summary, err := svc.StoreDeck(context.Background(), testUserID, fixtureDeckID, 5, []string{deck.BoardMain, deck.BoardSideboard, deck.BoardConsidering})

	require.NoError(t, err)
	assert.Equal(t, 3, summary.CardsMoved)
	assert.Equal(t, []int{1, 2, 4}, cards.movedIDs)
}

func TestStoreDeck_RejectsBadRequests(t *testing.T) {
	ctx := context.Background()
	cards, decks, storages := storeFixture()
	own := NewService(cards, storages, decks, &fakeResolver{})
	shared := NewService(&fakeCardService{}, storages, sharedDeckFixture(), &fakeResolver{})
	main := []string{deck.BoardMain}

	_, err := own.StoreDeck(ctx, testUserID, fixtureDeckID, 4, nil)
	assert.ErrorIs(t, err, ErrNoBoards)
	_, err = own.StoreDeck(ctx, testUserID, fixtureDeckID, 4, []string{"maybe"})
	assert.ErrorIs(t, err, ErrNoBoards)
	_, err = own.StoreDeck(ctx, testUserID, fixtureDeckID, 9, main)
	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
	_, err = own.StoreDeck(ctx, testUserID, "00000000-0000-0000-0000-000000000099", 4, main)
	assert.ErrorIs(t, err, deck.ErrNotFound)
	_, err = shared.StoreDeck(ctx, testUserID, fixtureDeckID, 4, main)
	assert.ErrorIs(t, err, deck.ErrNotFound)
	assert.Nil(t, cards.movedIDs)

	cards.moveErr = card.ErrStorageNotFound
	_, err = own.StoreDeck(ctx, testUserID, fixtureDeckID, 4, main)
	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
}

func TestHandler_StoreDeck(t *testing.T) {
	service := &fakeImportService{storeSummary: StoreSummary{CardsMoved: 12}}

	w := postJSON(setupRouter(service), "/deck/00000000-0000-0000-0000-000000000009/store", `{"storage_id": 4, "boards": ["main", "sideboard"]}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"cards_moved": 12}`, w.Body.String())
	assert.Equal(t, 4, service.lastStoreID)
	assert.Equal(t, []string{"main", "sideboard"}, service.lastStoreBoards)
	assert.Equal(t, "00000000-0000-0000-0000-000000000009", service.lastDeckID)
	assert.Equal(t, testUserID, service.lastUserID)
}

func TestHandler_StoreDeck_RejectsBadInput(t *testing.T) {
	router := setupRouter(&fakeImportService{})
	path := "/deck/00000000-0000-0000-0000-000000000009/store"
	assert.Equal(t, http.StatusBadRequest, postJSON(router, path, "").Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, path, `{"boards": ["main"]}`).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, path, `{"storage_id": 0, "boards": ["main"]}`).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, "/deck/nope/store", `{"storage_id": 4, "boards": ["main"]}`).Code)

	cases := map[error]int{
		deck.ErrNotFound:         http.StatusNotFound,
		ErrNoBoards:              http.StatusBadRequest,
		ErrTargetStorageNotFound: http.StatusBadRequest,
		errors.New("boom"):       http.StatusInternalServerError,
	}
	for storeErr, expected := range cases {
		w := postJSON(setupRouter(&fakeImportService{err: storeErr}), path, `{"storage_id": 4, "boards": ["main"]}`)
		assert.Equal(t, expected, w.Code, storeErr.Error())
	}
}
