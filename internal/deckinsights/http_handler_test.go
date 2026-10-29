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

	"Melrakkiie/Tamiyo/internal/deck"
)

const handlerDeckID = "00000000-0000-0000-0000-000000000007"

type fakeInsightsService struct {
	legality LegalityReport
	stats    DeckStats
	combos   DeckCombos
	history  DeckHistory
	err      error

	lastViewerID string
	lastDeckID   string
	lastReach    deck.Reach
	calls        int
}

func (f *fakeInsightsService) record(viewerID string, deckID string, reach deck.Reach) {
	f.calls++
	f.lastViewerID = viewerID
	f.lastDeckID = deckID
	f.lastReach = reach
}

func (f *fakeInsightsService) GetDeckLegality(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (LegalityReport, error) {
	f.record(viewerID, deckID, reach)
	return f.legality, f.err
}

func (f *fakeInsightsService) GetDeckStats(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (DeckStats, error) {
	f.record(viewerID, deckID, reach)
	return f.stats, f.err
}

func (f *fakeInsightsService) GetDeckCombos(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (DeckCombos, error) {
	f.record(viewerID, deckID, reach)
	return f.combos, f.err
}

// setupRouter registers the private routes behind a middleware that
// authenticates testUserID, and the shared routes without any.
func (f *fakeInsightsService) GetDeckHistory(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (DeckHistory, error) {
	f.record(viewerID, deckID, reach)
	return f.history, f.err
}

func setupRouter(service insightsService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(service)
	protected := router.Group("")
	protected.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Next()
	})
	handler.RegisterRoutes(protected)
	handler.RegisterSharedRoutes(router.Group(""))
	return router
}

// setupUnauthenticatedRouter registers both families with nobody signed in.
func setupUnauthenticatedRouter(service insightsService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(service)
	handler.RegisterRoutes(router.Group(""))
	handler.RegisterSharedRoutes(router.Group(""))
	return router
}

func get(router *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// family is a route family: where its routes live and who reads through it.
type family struct {
	name   string
	prefix string
	reach  deck.Reach
	viewer string
	// badIDStatus and badIDBody are the answer to a malformed deck id.
	badIDStatus int
	badIDBody   string
}

var families = []family{
	{name: "private", prefix: "/deck/", reach: deck.ReachOwn, viewer: testUserID,
		badIDStatus: http.StatusBadRequest, badIDBody: `{"error": "invalid id"}`},
	{name: "shared", prefix: "/shared/decks/", reach: deck.ReachShared, viewer: "",
		badIDStatus: http.StatusNotFound, badIDBody: `{"error": "deck not found"}`},
}

var insightRoutes = []string{"legality", "stats", "combos", "history"}

func TestRoutes_ReadTheDeckWithTheReachAndViewerOfTheirFamily(t *testing.T) {
	for _, f := range families {
		for _, route := range insightRoutes {
			t.Run(f.name+"/"+route, func(t *testing.T) {
				service := &fakeInsightsService{}

				w := get(setupRouter(service), f.prefix+handlerDeckID+"/"+route)

				require.Equal(t, http.StatusOK, w.Code)
				assert.Equal(t, 1, service.calls)
				assert.Equal(t, f.viewer, service.lastViewerID)
				assert.Equal(t, handlerDeckID, service.lastDeckID)
				assert.Equal(t, f.reach, service.lastReach)
			})
		}
	}
}

func TestRoutes_InvalidIDIsRejectedAsEachFamilyAlwaysDid(t *testing.T) {
	for _, f := range families {
		for _, route := range insightRoutes {
			t.Run(f.name+"/"+route, func(t *testing.T) {
				service := &fakeInsightsService{}

				w := get(setupRouter(service), f.prefix+"not-a-number/"+route)

				assert.Equal(t, f.badIDStatus, w.Code)
				assert.JSONEq(t, f.badIDBody, w.Body.String())
				assert.Zero(t, service.calls)
			})
		}
	}
}

func TestRoutes_EachSentinelMapsToTheSameStatusOnBothFamilies(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"unknown deck", deck.ErrNotFound, http.StatusNotFound, `{"error": "deck not found"}`},
		{"unknown format", ErrUnknownFormat, http.StatusBadRequest, `{"error": "` + ErrUnknownFormat.Error() + `"}`},
		{"combos unavailable", fmt.Errorf("%w: status 503", ErrCombosUnavailable), http.StatusBadGateway, `{"error": "commander spellbook is unavailable"}`},
		{"anything else", errUnexpected{}, http.StatusInternalServerError, ""},
	}
	for _, f := range families {
		for _, route := range insightRoutes {
			for _, c := range cases {
				t.Run(f.name+"/"+route+"/"+c.name, func(t *testing.T) {
					w := get(setupRouter(&fakeInsightsService{err: c.err}), f.prefix+handlerDeckID+"/"+route)

					assert.Equal(t, c.status, w.Code)
					if c.body != "" {
						assert.JSONEq(t, c.body, w.Body.String())
					}
				})
			}
		}
	}
}

func TestPrivateRoutes_RejectAnUnauthenticatedRequest(t *testing.T) {
	for _, route := range insightRoutes {
		t.Run(route, func(t *testing.T) {
			service := &fakeInsightsService{}

			w := get(setupUnauthenticatedRouter(service), "/deck/"+handlerDeckID+"/"+route)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.JSONEq(t, `{"error": "unauthenticated"}`, w.Body.String())
			assert.Zero(t, service.calls)
		})
	}
}

func TestSharedRoutes_AcceptAnAnonymousRequest(t *testing.T) {
	for _, route := range insightRoutes {
		t.Run(route, func(t *testing.T) {
			service := &fakeInsightsService{}

			w := get(setupUnauthenticatedRouter(service), "/shared/decks/"+handlerDeckID+"/"+route)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "", service.lastViewerID)
			assert.Equal(t, deck.ReachShared, service.lastReach)
		})
	}
}

func TestSharedRoutes_ReadAsTheSignedInViewerWhenThereIsOne(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := &fakeInsightsService{}
	viewer := router.Group("")
	viewer.Use(func(c *gin.Context) {
		c.Set("user_id", otherUserID)
		c.Next()
	})
	NewHandler(service).RegisterSharedRoutes(viewer)

	w := get(router, "/shared/decks/"+handlerDeckID+"/stats")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, otherUserID, service.lastViewerID)
}

func TestLegality_OnlyTheSharedRouteBlanksTheCardIDs(t *testing.T) {
	channel := legalPrinting(1, "Channel", "Sorcery", nil)
	channel.BannedFormats = []string{"commander"}
	router := setupRouter(NewService(readerWith(deckOf("commander", owned(channel, 7))), storeOf(channel), nil))

	shared := get(router, "/shared/decks/"+testDeckID+"/legality")
	private := get(router, "/deck/"+testDeckID+"/legality")

	require.Equal(t, http.StatusOK, shared.Code)
	require.Equal(t, http.StatusOK, private.Code)
	assert.NotContains(t, shared.Body.String(), "card_id")
	assert.Contains(t, shared.Body.String(), "banned in commander")
	assert.Contains(t, private.Body.String(), `"card_id": 7`)
}

func TestGetDeckStats_ReturnsStatsOnSuccess(t *testing.T) {
	for _, f := range families {
		t.Run(f.name, func(t *testing.T) {
			w := get(setupRouter(&fakeInsightsService{stats: DeckStats{CardCount: 42}}), f.prefix+handlerDeckID+"/stats")

			require.Equal(t, http.StatusOK, w.Code)
			var got DeckStats
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, 42, got.CardCount)
		})
	}
}

func TestGetDeckCombos_ReturnsTheCombosOnSuccess(t *testing.T) {
	for _, f := range families {
		t.Run(f.name, func(t *testing.T) {
			service := &fakeInsightsService{combos: DeckCombos{Combos: []Combo{{
				ID: "1414-2730", Cards: []string{"Niv-Mizzet, Parun", "Curiosity"}, Results: []string{"Infinite damage"},
				URL: "https://commanderspellbook.com/combo/1414-2730/",
			}}}}

			w := get(setupRouter(service), f.prefix+handlerDeckID+"/combos")

			require.Equal(t, http.StatusOK, w.Code)
			assert.JSONEq(t, `{"combos": [{
				"id": "1414-2730",
				"cards": ["Niv-Mizzet, Parun", "Curiosity"],
				"results": ["Infinite damage"],
				"url": "https://commanderspellbook.com/combo/1414-2730/"
			}]}`, w.Body.String())
		})
	}
}

type errUnexpected struct{}

func (errUnexpected) Error() string { return "something went wrong" }

func TestGetDeckHistory_ReturnsThePrintingOnlyOnAnnotatedLines(t *testing.T) {
	for _, f := range families {
		t.Run(f.name, func(t *testing.T) {
			service := &fakeInsightsService{history: DeckHistory{Days: []HistoryDay{{Date: "2026-10-29", Boards: []HistoryBoard{{Board: "main", Changes: []HistoryChange{
				{Name: "Sol Ring", Count: 2, SetCode: ptr("c21"), CollectorNumber: ptr("263"), Foil: ptr(true)},
				{Name: "Island", Count: -1},
			}}}}}}}

			w := get(setupRouter(service), f.prefix+handlerDeckID+"/history")

			require.Equal(t, http.StatusOK, w.Code)
			assert.JSONEq(t, `{"days": [{"date": "2026-10-29", "boards": [{"board": "main", "changes": [
				{"name": "Sol Ring", "count": 2, "set_code": "c21", "collector_number": "263", "foil": true},
				{"name": "Island", "count": -1}
			]}]}]}`, w.Body.String())
		})
	}
}

func ptr[T any](v T) *T { return &v }
