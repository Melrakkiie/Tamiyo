package deckinsights

import (
	"context"
	"encoding/json"
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
	err      error

	lastUserID string
	lastDeckID int
}

func (f *fakeInsightsService) GetDeckLegality(ctx context.Context, userID string, deckID int) (LegalityReport, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.legality, f.err
}

func (f *fakeInsightsService) GetDeckStats(ctx context.Context, userID string, deckID int) (DeckStats, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.stats, f.err
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

	req := httptest.NewRequest(http.MethodGet, "/deck/7/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got LegalityReport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Legal)
	assert.Equal(t, testUserID, service.lastUserID)
	assert.Equal(t, 7, service.lastDeckID)
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

	req := httptest.NewRequest(http.MethodGet, "/deck/999/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDeckLegality_UnknownFormatReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: ErrUnknownFormat})

	req := httptest.NewRequest(http.MethodGet, "/deck/1/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetDeckLegality_ScryfallFailureReturnsBadGateway(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: ErrScryfallUnavailable})

	req := httptest.NewRequest(http.MethodGet, "/deck/1/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestGetDeckLegality_UnknownErrorReturnsInternalServerError(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: assertAnError{}})

	req := httptest.NewRequest(http.MethodGet, "/deck/1/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetDeckStats_ReturnsStatsOnSuccess(t *testing.T) {
	service := &fakeInsightsService{stats: DeckStats{CardCount: 42}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/7/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got DeckStats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 42, got.CardCount)
}

func TestGetDeckStats_UnknownDeckReturnsNotFound(t *testing.T) {
	router := setupRouter(&fakeInsightsService{err: ErrDeckNotFound})

	req := httptest.NewRequest(http.MethodGet, "/deck/999/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDeckLegality_ReturnsUnauthorizedWhenNotAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(&fakeInsightsService{}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/deck/1/legality", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "something went wrong" }
