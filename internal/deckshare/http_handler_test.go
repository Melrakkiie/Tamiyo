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
	compared Comparison
	err      error

	lastUserID  string
	lastDeckID  string
	lastOtherID string
}

func (f *fakeSharedService) CompareDecks(ctx context.Context, userID string, deckID string, otherID string) (Comparison, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	f.lastOtherID = otherID
	return f.compared, f.err
}

func (f *fakeSharedService) GetSharedDeck(ctx context.Context, deckID string) (SharedDeck, error) {
	f.lastDeckID = deckID
	return f.shared, f.err
}

func (f *fakeSharedService) GetSharedDeckLegality(ctx context.Context, deckID string) (deckinsights.LegalityReport, error) {
	f.lastDeckID = deckID
	return f.legality, f.err
}

func (f *fakeSharedService) GetSharedDeckStats(ctx context.Context, deckID string) (deckinsights.DeckStats, error) {
	f.lastDeckID = deckID
	return f.stats, f.err
}

func setupRouter(service sharedDeckService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	return router
}

func setupProtectedRouter(service sharedDeckService, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("user_id", userID)
		}
		c.Next()
	})
	NewHandler(service).RegisterProtectedRoutes(router)
	return router
}

func get(router *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestGetSharedDeck_ReturnsDeckOwnerAndCardsWithoutAuthentication(t *testing.T) {
	service := &fakeSharedService{shared: SharedDeck{
		Deck:  deck.Deck{ID: deckID, Name: "Otters", Format: "commander", Visibility: deck.VisibilityUnlisted, Added: time.Now(), Updated: time.Now()},
		Owner: Owner{ID: ownerID, DisplayName: strPtr("Alice")},
		Cards: []Card{{Name: "Island", ScryfallID: "i", Quantity: 3, Tags: []string{"Terrain"}}, {Name: "Tamiyo", ScryfallID: "t", Quantity: 1, Commander: true}},
	}}
	router := setupRouter(service)

	w := get(router, "/shared/decks/"+deckID)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, deckID, service.lastDeckID)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	gotDeck := got["deck"].(map[string]any)
	assert.Equal(t, "Otters", gotDeck["name"])
	assert.Equal(t, deckID, gotDeck["id"])
	assert.Equal(t, float64(4), gotDeck["card_count"])
	assert.Equal(t, "Alice", got["owner"].(map[string]any)["display_name"])
	cards := got["cards"].([]any)
	require.Len(t, cards, 2)
	assert.Equal(t, true, cards[1].(map[string]any)["commander"])
	assert.Equal(t, []any{"Terrain"}, cards[0].(map[string]any)["tags"])
	assert.Equal(t, []any{}, cards[1].(map[string]any)["tags"])
	assert.NotContains(t, cards[0], "storage_id")
	assert.NotContains(t, cards[0], "proxy")
}

func TestGetSharedDeck_AcceptsAnUppercaseShareID(t *testing.T) {
	service := &fakeSharedService{}
	router := setupRouter(service)

	w := get(router, "/shared/decks/ABCDEF12-2222-2222-2222-222222222222")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "abcdef12-2222-2222-2222-222222222222", service.lastDeckID)
}

func TestGetSharedDeck_MalformedShareIDReturnsNotFound(t *testing.T) {
	service := &fakeSharedService{}
	router := setupRouter(service)

	w := get(router, "/shared/decks/42")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, service.lastDeckID)
}

func TestGetSharedDeck_HiddenDeckReturnsNotFound(t *testing.T) {
	router := setupRouter(&fakeSharedService{err: ErrNotFound})

	w := get(router, "/shared/decks/"+deckID)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetSharedDeckLegality_ReturnsReport(t *testing.T) {
	router := setupRouter(&fakeSharedService{legality: deckinsights.LegalityReport{Format: "commander", Legal: true}})

	w := get(router, "/shared/decks/"+deckID+"/legality")

	require.Equal(t, http.StatusOK, w.Code)
	var got deckinsights.LegalityReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Legal)
}

func TestGetSharedDeckLegality_UnknownFormatReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeSharedService{err: deckinsights.ErrUnknownFormat})

	w := get(router, "/shared/decks/"+deckID+"/legality")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetSharedDeckStats_ReturnsStats(t *testing.T) {
	router := setupRouter(&fakeSharedService{stats: deckinsights.DeckStats{CardCount: 100}})

	w := get(router, "/shared/decks/"+deckID+"/stats")

	require.Equal(t, http.StatusOK, w.Code)
	var got deckinsights.DeckStats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 100, got.CardCount)
}

func TestGetSharedDeckStats_ScryfallDownReturnsBadGateway(t *testing.T) {
	router := setupRouter(&fakeSharedService{err: deckinsights.ErrScryfallUnavailable})

	w := get(router, "/shared/decks/"+deckID+"/stats")

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestGetSharedDeck_CountsOnlyTheMainBoard(t *testing.T) {
	service := &fakeSharedService{shared: SharedDeck{
		Deck: deck.Deck{ID: deckID, Name: "Otters", Added: time.Now(), Updated: time.Now()},
		Cards: []Card{
			{Name: "Island", ScryfallID: "i", Quantity: 3, Board: deck.BoardMain},
			{Name: "Duress", ScryfallID: "d", Quantity: 2, Board: deck.BoardSideboard},
			{Name: "Opt", ScryfallID: "o", Quantity: 1, Board: deck.BoardConsidering},
		},
	}}

	w := get(setupRouter(service), "/shared/decks/"+deckID)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, float64(3), got["deck"].(map[string]any)["card_count"])
	assert.Equal(t, "sideboard", got["cards"].([]any)[1].(map[string]any)["board"])
}
