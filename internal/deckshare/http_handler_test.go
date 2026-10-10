package deckshare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	public      []deck.PublicDeck
	publicTotal int
	owned       []OwnedCard
	lastFilter  deck.PublicFilter

	likeStatus deck.LikeStatus
	lastLike   string
	liked      []deck.PublicDeck
	lastPage   int
	lastLimit  int
}

func (f *fakeSharedService) recordLike(action string, userID string, deckID string) (deck.LikeStatus, error) {
	f.lastLike = action
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.likeStatus, f.err
}

func (f *fakeSharedService) LikeStatus(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error) {
	return f.recordLike("status", userID, deckID)
}

func (f *fakeSharedService) LikeDeck(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error) {
	return f.recordLike("like", userID, deckID)
}

func (f *fakeSharedService) UnlikeDeck(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error) {
	return f.recordLike("unlike", userID, deckID)
}

func (f *fakeSharedService) LikedDecks(ctx context.Context, userID string, page int, limit int) ([]deck.PublicDeck, int, error) {
	f.lastUserID = userID
	f.lastPage = page
	f.lastLimit = limit
	return f.liked, len(f.liked), f.err
}

func (f *fakeSharedService) CompareDecks(ctx context.Context, userID string, deckID string, otherID string) (Comparison, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	f.lastOtherID = otherID
	return f.compared, f.err
}

func (f *fakeSharedService) CollectionOwnership(ctx context.Context, viewerID string, deckID string) ([]OwnedCard, error) {
	f.lastUserID = viewerID
	f.lastDeckID = deckID
	return f.owned, f.err
}

func (f *fakeSharedService) BrowsePublicDecks(ctx context.Context, filter deck.PublicFilter) ([]deck.PublicDeck, int, error) {
	f.lastFilter = filter
	return f.public, f.publicTotal, f.err
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
		Deck:  deck.Deck{ID: deckID, Name: "Otters", Format: "commander", Visibility: deck.VisibilityUnlisted, LikesCount: 7, Added: time.Now(), Updated: time.Now()},
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
	assert.Equal(t, float64(7), gotDeck["likes_count"])
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

func TestBrowsePublicDecks_ReturnsAPageWithoutAuthentication(t *testing.T) {
	updated := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	service := &fakeSharedService{
		public: []deck.PublicDeck{{
			ID: deckID, Name: "Otters", Format: "commander", ColorIdentity: "UG", CardCount: 100,
			CommanderName: strPtr("Loot"), OwnerID: ownerID, OwnerDisplayName: strPtr("Alice"),
			Added: updated, Updated: updated,
		}},
		publicTotal: 30,
	}

	w := get(setupRouter(service), "/shared/decks?page=2&limit=10")

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, float64(2), got["page"])
	assert.Equal(t, float64(3), got["total_pages"])
	first := got["data"].([]any)[0].(map[string]any)
	assert.Equal(t, "Otters", first["name"])
	assert.Equal(t, "UG", first["color_identity"])
	assert.Equal(t, "Loot", first["commander_name"])
	assert.Equal(t, "Alice", first["owner"].(map[string]any)["display_name"])
	assert.Equal(t, deck.PublicFilter{SortField: "updated", SortDesc: true, ColorMode: deck.ColorModeExact, Page: 2, Limit: 10}, service.lastFilter)
}

func TestBrowsePublicDecks_ParsesTheFilters(t *testing.T) {
	service := &fakeSharedService{}

	w := get(setupRouter(service), "/shared/decks?q=%20otters%20&format=commander&commander=loot&card=sol%20ring&owner=ali&colors=gub&color_mode=within&color_count=2&sort=-card_count")

	require.Equal(t, http.StatusOK, w.Code)
	count := 2
	assert.Equal(t, deck.PublicFilter{
		Name: "otters", Format: "commander", Commander: "loot", Card: "sol ring", Owner: "ali",
		Colors: []string{"G", "U", "B"}, ColorMode: deck.ColorModeWithin, ColorCount: &count,
		SortField: "card_count", SortDesc: true, Page: 1, Limit: 24,
	}, service.lastFilter)
	assert.Equal(t, []any{}, decodeData(t, w))
}

func TestBrowsePublicDecks_Colorless(t *testing.T) {
	service := &fakeSharedService{}

	w := get(setupRouter(service), "/shared/decks?colors=c")

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, service.lastFilter.Colorless)
	assert.Empty(t, service.lastFilter.Colors)
}

func TestBrowsePublicDecks_RejectsInvalidQueries(t *testing.T) {
	for _, query := range []string{
		"page=0", "limit=101", "limit=x", "colors=WX", "color_mode=some", "color_count=6",
		"sort=owner", "q=" + strings.Repeat("a", 101),
	} {
		w := get(setupRouter(&fakeSharedService{}), "/shared/decks?"+query)
		assert.Equal(t, http.StatusBadRequest, w.Code, query)
	}
}

func decodeData(t *testing.T, w *httptest.ResponseRecorder) any {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	return got["data"]
}

func TestCollectionOwnership_ReturnsTheViewersCounts(t *testing.T) {
	service := &fakeSharedService{owned: []OwnedCard{{Name: "Sol Ring", Owned: 2}, {Name: "Island", Owned: 0}}}
	router := setupProtectedRouter(service, "33333333-3333-3333-3333-333333333333")

	w := get(router, "/deck/"+deckID+"/ownership")

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"cards":[{"name":"Sol Ring","owned":2},{"name":"Island","owned":0}]}`, w.Body.String())
	assert.Equal(t, "33333333-3333-3333-3333-333333333333", service.lastUserID)
	assert.Equal(t, deckID, service.lastDeckID)
}

func TestCollectionOwnership_Errors(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, get(setupProtectedRouter(&fakeSharedService{}, ""), "/deck/"+deckID+"/ownership").Code)
	assert.Equal(t, http.StatusNotFound, get(setupProtectedRouter(&fakeSharedService{err: ErrNotFound}, "33333333-3333-3333-3333-333333333333"), "/deck/"+deckID+"/ownership").Code)
	assert.Equal(t, http.StatusNotFound, get(setupProtectedRouter(&fakeSharedService{}, "33333333-3333-3333-3333-333333333333"), "/deck/nope/ownership").Code)
}
