package bulk

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/storage"
)

func transferFixture() (*fakeCardService, *fakeStorageService) {
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 4, Name: "Box", Type: "box"}, {ID: 5, Name: "Binder", Type: "binder"}}}
	return &fakeCardService{}, storages
}

func TestTransferStorage_MovesEveryCardOfTheSource(t *testing.T) {
	cards, storages := transferFixture()
	svc := newTestService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	summary, err := svc.TransferStorage(context.Background(), testUserID, 4, 5)

	require.NoError(t, err)
	assert.Equal(t, StoreSummary{CardsMoved: 7}, summary)
	assert.Equal(t, 4, cards.movedFrom)
	assert.Equal(t, 5, cards.movedStorage)
}

func TestTransferStorage_RejectsBadRequests(t *testing.T) {
	ctx := context.Background()
	cards, storages := transferFixture()
	svc := newTestService(cards, storages, &fakeDeckService{}, &fakeResolver{})

	_, err := svc.TransferStorage(ctx, testUserID, 4, 4)
	assert.ErrorIs(t, err, ErrSameStorage)
	_, err = svc.TransferStorage(ctx, testUserID, 9, 5)
	assert.ErrorIs(t, err, ErrStorageNotFound)
	_, err = svc.TransferStorage(ctx, testUserID, 4, 9)
	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
	assert.False(t, cards.moveCalled)

	cards.moveErr = card.ErrStorageNotFound
	_, err = svc.TransferStorage(ctx, testUserID, 4, 5)
	assert.ErrorIs(t, err, ErrTargetStorageNotFound)

	boom := errors.New("boom")
	cards.moveErr = boom
	_, err = svc.TransferStorage(ctx, testUserID, 4, 5)
	assert.ErrorIs(t, err, boom)
}

func TestHandler_TransferStorage(t *testing.T) {
	service := &fakeImportService{storeSummary: StoreSummary{CardsMoved: 12}}

	w := postJSON(setupRouter(service), "/storage/4/transfer", `{"storage_id": 5}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"cards_moved": 12}`, w.Body.String())
	assert.Equal(t, 4, service.lastFromStorage)
	assert.Equal(t, 5, service.lastStoreID)
	assert.Equal(t, testUserID, service.lastUserID)
}

func TestHandler_TransferStorage_RejectsBadInput(t *testing.T) {
	router := setupRouter(&fakeImportService{})
	path := "/storage/4/transfer"
	assert.Equal(t, http.StatusBadRequest, postJSON(router, path, "").Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, path, `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, path, `{"storage_id": 0}`).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, "/storage/nope/transfer", `{"storage_id": 5}`).Code)

	cases := map[error]int{
		ErrStorageNotFound:       http.StatusNotFound,
		ErrSameStorage:           http.StatusBadRequest,
		ErrTargetStorageNotFound: http.StatusBadRequest,
		errors.New("boom"):       http.StatusInternalServerError,
	}
	for transferErr, expected := range cases {
		w := postJSON(setupRouter(&fakeImportService{err: transferErr}), path, `{"storage_id": 5}`)
		assert.Equal(t, expected, w.Code, transferErr.Error())
	}
}
