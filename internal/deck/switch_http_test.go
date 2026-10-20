package deck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSwitches struct {
	switchSummary  ReplacementSummary
	switchErr      error
	lastSwitch     string
	lastSelection  CardSelection
	lastSwitchedTo bool
	lastPrintings  []PrintingChoice
}

func (f *fakeSwitches) SwitchFinish(ctx context.Context, userID string, deckID string, cards CardSelection, foil bool) (ReplacementSummary, error) {
	f.lastSwitch, f.lastSelection, f.lastSwitchedTo = "foil", cards, foil
	return f.switchSummary, f.switchErr
}

func (f *fakeSwitches) SwitchProxy(ctx context.Context, userID string, deckID string, cards CardSelection, proxy bool) (ReplacementSummary, error) {
	f.lastSwitch, f.lastSelection, f.lastSwitchedTo = "proxy", cards, proxy
	return f.switchSummary, f.switchErr
}

func (f *fakeSwitches) SplitPrintings(ctx context.Context, userID string, deckID string, cards CardSelection, printings []PrintingChoice) (ReplacementSummary, error) {
	f.lastSwitch, f.lastSelection, f.lastPrintings = "printings", cards, printings
	return f.switchSummary, f.switchErr
}

const switchDeckURL = "/deck/00000000-0000-0000-0000-000000000009/"

func postSwitch(service *fakeService, route string, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, switchDeckURL+route, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setupRouter(service).ServeHTTP(w, req)
	return w
}

func TestHandler_SwitchDeckCards(t *testing.T) {
	for _, field := range []string{"foil", "proxy"} {
		t.Run(field, func(t *testing.T) {
			service := &fakeService{fakeSwitches: fakeSwitches{switchSummary: ReplacementSummary{CardsLinked: 1, CardsPending: 2, CardsRemoved: 3}}}
			post := func(body string) *httptest.ResponseRecorder {
				return postSwitch(service, "switch-"+field, strings.ReplaceAll(body, "FIELD", field))
			}

			w := post(`{"card_ids": [2, 3], "FIELD": true}`)
			require.Equal(t, http.StatusOK, w.Code)
			assert.JSONEq(t, `{"cards_created": 0, "cards_linked": 1, "cards_pending": 2, "cards_skipped": 0, "storages_created": 0, "decks_created": 0, "cards_removed": 3}`, w.Body.String())
			assert.Equal(t, field, service.lastSwitch)
			assert.Equal(t, CardSelection{CardIDs: []int{2, 3}}, service.lastSelection)
			assert.True(t, service.lastSwitchedTo)

			assert.Equal(t, http.StatusOK, post(`{"pending_ids": [8], "FIELD": false}`).Code)
			assert.Equal(t, CardSelection{PendingIDs: []int{8}}, service.lastSelection)
			assert.False(t, service.lastSwitchedTo)

			assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2]}`).Code)
			assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [], "FIELD": false}`).Code)
			assert.Equal(t, http.StatusBadRequest, post(`nope`).Code)

			service.switchErr = ErrCardsNotInDeck
			assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [9], "FIELD": true}`).Code)
			service.switchErr = ErrNotFound
			assert.Equal(t, http.StatusNotFound, post(`{"card_ids": [9], "FIELD": true}`).Code)
		})
	}
}

func TestHandler_SwitchDeckCards_ReturnsTheErrorWithoutASummary(t *testing.T) {
	service := &fakeService{fakeSwitches: fakeSwitches{switchSummary: ReplacementSummary{CardsLinked: 1}, switchErr: assert.AnError}}

	w := postSwitch(service, "switch-foil", `{"card_ids": [2], "foil": true}`)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "cards_linked")
}

func TestHandler_SplitPrintings(t *testing.T) {
	service := &fakeService{fakeSwitches: fakeSwitches{switchSummary: ReplacementSummary{CardsLinked: 1, CardsRemoved: 1}}}
	post := func(body string) *httptest.ResponseRecorder { return postSwitch(service, "split-printings", body) }

	w := post(`{"card_ids": [2], "pending_ids": [8], "printings": [
		{"scryfall_id": "a1", "set_code": "one", "collector_number": "263", "foil": true},
		{"scryfall_id": "b2", "set_code": "neo", "collector_number": "294"}]}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "printings", service.lastSwitch)
	assert.Equal(t, CardSelection{CardIDs: []int{2}, PendingIDs: []int{8}}, service.lastSelection)
	assert.Equal(t, []PrintingChoice{
		{ScryfallID: "a1", SetCode: "one", CollectorNumber: "263", Foil: true},
		{ScryfallID: "b2", SetCode: "neo", CollectorNumber: "294"},
	}, service.lastPrintings)
	assert.Contains(t, w.Body.String(), `"cards_linked":1`)

	assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2], "printings": []}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(`{"printings": [{"scryfall_id": "x"}]}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2], "printings": [{"set_code": "one"}]}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(`nope`).Code)

	service.switchErr = ErrSplitMismatch
	assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2], "printings": [{"scryfall_id": "x"}]}`).Code)
	service.switchErr = ErrCardsNotInDeck
	assert.Equal(t, http.StatusBadRequest, post(`{"card_ids": [2], "printings": [{"scryfall_id": "x"}]}`).Code)
	service.switchErr = ErrNotFound
	assert.Equal(t, http.StatusNotFound, post(`{"card_ids": [2], "printings": [{"scryfall_id": "x"}]}`).Code)
}
