package deckinsights

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeInsightsService struct {
	legality LegalityReport
	stats    DeckStats
	combos   DeckCombos
	err      error

	lastUserID string
	lastDeckID string
}

func (f *fakeInsightsService) GetDeckLegality(ctx context.Context, userID string, deckID string) (LegalityReport, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.legality, f.err
}

func (f *fakeInsightsService) GetDeckStats(ctx context.Context, userID string, deckID string) (DeckStats, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.stats, f.err
}

func (f *fakeInsightsService) GetDeckCombos(ctx context.Context, userID string, deckID string) (DeckCombos, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.combos, f.err
}

func setupRouter(service insightsService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Next()
	})
	NewHandler(service).RegisterRoutes(router)
	return router
}

func TestGetDeckLegality_ReturnsReportOnSuccess(t *testing.T) {
	service := &fakeInsightsService{legality: LegalityReport{Format: "commander", Legal: true}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000007/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got LegalityReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Legal)
	assert.Equal(t, testUserID, service.lastUserID)
	assert.Equal(t, "00000000-0000-0000-0000-000000000007", service.lastDeckID)
}

func TestGetDeckLegality_InvalidIDReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeInsightsService{})

	req := httptest.NewRequest(http.MethodGet, "/deck/not-a-number/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetDeckLegality_UnknownDeckReturnsNotFound(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: ErrDeckNotFound})

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000999/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDeckLegality_UnknownFormatReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: ErrUnknownFormat})

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetDeckLegality_UnknownErrorReturnsInternalServerError(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: assertAnError{}})

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetDeckStats_ReturnsStatsOnSuccess(t *testing.T) {
	service := &fakeInsightsService{stats: DeckStats{CardCount: 42}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000007/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got DeckStats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 42, got.CardCount)
}

func TestGetDeckStats_UnknownDeckReturnsNotFound(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: ErrDeckNotFound})

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000999/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDeckCombos_ReturnsTheCombosOnSuccess(t *testing.T) {
	service := &fakeInsightsService{combos: DeckCombos{Combos: []Combo{{
		ID: "1414-2730", Cards: []string{"Niv-Mizzet, Parun", "Curiosity"}, Results: []string{"Infinite damage"},
		URL: "https://commanderspellbook.com/combo/1414-2730/",
	}}}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000007/combos", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"combos": [{
		"id": "1414-2730",
		"cards": ["Niv-Mizzet, Parun", "Curiosity"],
		"results": ["Infinite damage"],
		"url": "https://commanderspellbook.com/combo/1414-2730/"
	}]}`, w.Body.String())
	assert.Equal(t, testUserID, service.lastUserID)
	assert.Equal(t, "00000000-0000-0000-0000-000000000007", service.lastDeckID)
}

func TestGetDeckCombos_UnavailableSpellbookReturnsBadGateway(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: fmt.Errorf("%w: status 503", ErrCombosUnavailable)})

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/combos", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.JSONEq(t, `{"error": "commander spellbook is unavailable"}`, w.Body.String())
}

func TestGetDeckCombos_UnknownDeckReturnsNotFound(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: ErrDeckNotFound})

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000999/combos", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDeckLegality_ReturnsUnauthorizedWhenNotAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(&fakeInsightsService{}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "something went wrong" }
