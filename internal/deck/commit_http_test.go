package deck

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const commitURL = "/deck/00000000-0000-0000-0000-000000000009/pending/commit"

func (f *fakeService) CommitPendingCards(ctx context.Context, userID string, deckID string, req PendingCommit) (PendingCommitSummary, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	f.lastCommit = &req
	return f.commitSummary, f.commitErr
}

func postCommit(service *fakeService, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(http.MethodPost, commitURL, nil)
	} else {
		req = httptest.NewRequest(http.MethodPost, commitURL, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	setupRouter(service).ServeHTTP(w, req)
	return w
}

func TestHandler_CommitPendingCards_PassesTheRequestAndReturnsTheSummary(t *testing.T) {
	service := &fakeService{commitSummary: PendingCommitSummary{CardsCreated: 3}}

	w := postCommit(service, `{"storage_id": 4, "pending_id": 3, "quantity": 2}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"cards_created": 3}`, w.Body.String())
	assert.Equal(t, testUserID, service.lastUserID)
	assert.Equal(t, "00000000-0000-0000-0000-000000000009", service.lastDeckID)
	assert.Equal(t, &PendingCommit{StorageID: intPtr(4), PendingID: intPtr(3), Quantity: intPtr(2)}, service.lastCommit)
}

func TestHandler_CommitPendingCards_WorksWithoutABody(t *testing.T) {
	service := &fakeService{}

	w := postCommit(service, "")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, &PendingCommit{}, service.lastCommit)
}

func TestHandler_CommitPendingCards_RejectsBadBodies(t *testing.T) {
	for _, body := range []string{`{"storage_id": 0}`, `{"pending_id": -1}`, `{"quantity": 5}`, `{"pending_id": 3, "quantity": 0}`, `not json`} {
		service := &fakeService{}

		w := postCommit(service, body)

		assert.Equal(t, http.StatusBadRequest, w.Code, body)
		assert.Nil(t, service.lastCommit, body)
	}
}

func TestHandler_CommitPendingCards_ReturnsTheErrorWithoutASummary(t *testing.T) {
	for commitErr, status := range map[error]int{
		ErrNotFound:              http.StatusNotFound,
		ErrPendingCardNotFound:   http.StatusNotFound,
		ErrTargetStorageNotFound: http.StatusBadRequest,
	} {
		service := &fakeService{commitErr: commitErr, commitSummary: PendingCommitSummary{CardsCreated: 2}}

		w := postCommit(service, "")

		assert.Equal(t, status, w.Code, commitErr.Error())
		assert.NotContains(t, w.Body.String(), "cards_created", commitErr.Error())
	}
}
