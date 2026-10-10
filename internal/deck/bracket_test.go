package deck

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int { return &i }

func sendDeckJSON(router http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHandler_CreateDeck_WithABracket(t *testing.T) {
	service := &fakeService{}

	w := sendDeckJSON(setupRouter(service), http.MethodPost, "/deck", `{"name": "Otters", "format": "commander", "bracket": 3}`)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, intPtr(3), service.lastCreatedDeck.Bracket)
	var response deckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, intPtr(3), response.Bracket)
}

func TestHandler_UpdateDeck_SetsOrClearsTheBracket(t *testing.T) {
	service := &fakeService{updateDeck: Deck{ID: "00000000-0000-0000-0000-000000000001", Name: "Otters", Format: "commander"}}
	router := setupRouter(service)

	w := sendDeckJSON(router, http.MethodPatch, "/deck/00000000-0000-0000-0000-000000000001", `{"bracket": 4}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, intPtr(4), service.lastUpdateRequest.Bracket)
	assert.Contains(t, w.Body.String(), `"bracket": null`)

	w = sendDeckJSON(router, http.MethodPatch, "/deck/00000000-0000-0000-0000-000000000001", `{"clear_bracket": true}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, service.lastUpdateRequest.ClearBracket)

	for _, body := range []string{`{"bracket": 0}`, `{"bracket": 6}`, `{"bracket": "3"}`} {
		w = sendDeckJSON(router, http.MethodPatch, "/deck/00000000-0000-0000-0000-000000000001", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
	}
	w = sendDeckJSON(router, http.MethodPost, "/deck", `{"name": "Otters", "format": "commander", "bracket": 9}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateDeckRequest_AppliesTheBracket(t *testing.T) {
	current := Deck{Bracket: intPtr(2)}

	assert.Equal(t, intPtr(5), updateDeckRequest{Bracket: intPtr(5)}.applyTo(current).Bracket)
	assert.Equal(t, intPtr(2), updateDeckRequest{}.applyTo(current).Bracket)
	assert.Nil(t, updateDeckRequest{ClearBracket: true, Bracket: intPtr(4)}.applyTo(current).Bracket)
}
