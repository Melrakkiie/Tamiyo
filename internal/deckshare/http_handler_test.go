package deckshare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/deckinsights"
)

type fakeSharedService struct {
	shared   SharedDeck
	legality deckinsights.LegalityReport
	stats    deckinsights.DeckStats
	err      error

	lastShareID string
}

func (f *fakeSharedService) GetSharedDeck(ctx context.Context, shareID string) (SharedDeck, error) {
	f.lastShareID = shareID
	return f.shared, f.err
}

func (f *fakeSharedService) GetSharedDeckLegality(ctx context.Context, shareID string) (deckinsights.LegalityReport, error) {
	f.lastShareID = shareID
	return f.legality, f.err
}

func (f *fakeSharedService) GetSharedDeckStats(ctx context.Context, shareID string) (deckinsights.DeckStats, error) {
	f.lastShareID = shareID
	return f.stats, f.err
}

func setupRouter(service sharedDeckService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	return router
}

func get(router *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestGetSharedDeck_ReturnsDeckOwnerAndCardsWithoutAuthentication(t *testing.T) {
	service := &fakeSharedService{shared: SharedDeck{
		Deck:  deck.Deck{ID: 9, Name: "Otters", Format: "commander", Visibility: deck.VisibilityUnlisted, ShareID: shareID, Added: time.Now(), Updated: time.Now()},
		Owner: Owner{ID: ownerID, DisplayName: strPtr("Alice")},
		Cards: []Card{{Name: "Island", ScryfallID: "i", Quantity: 3}, {Name: "Tamiyo", ScryfallID: "t", Quantity: 1, Commander: true}},
	}}
	router := setupRouter(service)

	w := get(router, "/shared/decks/"+shareID)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, shareID, service.lastShareID)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	gotDeck := got["deck"].(map[string]any)
	assert.Equal(t, "Otters", gotDeck["name"])
	assert.Equal(t, shareID, gotDeck["share_id"])
	assert.Equal(t, float64(4), gotDeck["card_count"])
	assert.NotContains(t, gotDeck, "id")
	assert.Equal(t, "Alice", got["owner"].(map[string]any)["display_name"])
	cards := got["cards"].([]any)
	require.Len(t, cards, 2)
	assert.Equal(t, true, cards[1].(map[string]any)["commander"])
	assert.NotContains(t, cards[0], "storage_id")
	assert.NotContains(t, cards[0], "proxy")
}

func TestGetSharedDeck_AcceptsAnUppercaseShareID(t *testing.T) {
	service := &fakeSharedService{}
	router := setupRouter(service)

	w := get(router, "/shared/decks/ABCDEF12-2222-2222-2222-222222222222")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "abcdef12-2222-2222-2222-222222222222", service.lastShareID)
}

func TestGetSharedDeck_MalformedShareIDReturnsNotFound(t *testing.T) {
	service := &fakeSharedService{}
	router := setupRouter(service)

	w := get(router, "/shared/decks/42")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, service.lastShareID)
}

func TestGetSharedDeck_HiddenDeckReturnsNotFound(t *testing.T) {
	router := setupRouter(&fakeSharedService{err: ErrNotFound})

	w := get(router, "/shared/decks/"+shareID)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetSharedDeckLegality_ReturnsReport(t *testing.T) {
	router := setupRouter(&fakeSharedService{legality: deckinsights.LegalityReport{Format: "commander", Legal: true}})

	w := get(router, "/shared/decks/"+shareID+"/legality")

	require.Equal(t, http.StatusOK, w.Code)
	var got deckinsights.LegalityReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Legal)
}

func TestGetSharedDeckLegality_UnknownFormatReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeSharedService{err: deckinsights.ErrUnknownFormat})

	w := get(router, "/shared/decks/"+shareID+"/legality")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetSharedDeckStats_ReturnsStats(t *testing.T) {
	router := setupRouter(&fakeSharedService{stats: deckinsights.DeckStats{CardCount: 100}})

	w := get(router, "/shared/decks/"+shareID+"/stats")

	require.Equal(t, http.StatusOK, w.Code)
	var got deckinsights.DeckStats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 100, got.CardCount)
}

func TestGetSharedDeckStats_ScryfallDownReturnsBadGateway(t *testing.T) {
	router := setupRouter(&fakeSharedService{err: deckinsights.ErrScryfallUnavailable})

	w := get(router, "/shared/decks/"+shareID+"/stats")

	assert.Equal(t, http.StatusBadGateway, w.Code)
}
