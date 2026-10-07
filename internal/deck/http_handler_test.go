package deck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeService struct {
	decks       []Deck
	getAllTotal int
	getAllErr   error
	lastUserID  string
	lastFilter  Filter

	getDeck    Deck
	getDeckErr error

	createErr error

	updateDeck        Deck
	updateErr         error
	lastUpdateRequest updateDeckRequest
	lastCreatedDeck   Deck

	deleteErr error

	getDeckCards          []DeckCard
	getDeckCardsErr       error
	lastDeckCardsSort     string
	lastDeckCardsSortDesc bool

	putCardErr    error
	removeCardErr error

	pending          []PendingCard
	pendingErr       error
	addedPending     PendingCard
	removePendingErr error
}

func (f *fakeService) GetPendingCards(ctx context.Context, userID string, deckID int) ([]PendingCard, error) {
	f.lastUserID = userID
	return f.pending, f.pendingErr
}

func (f *fakeService) AddPendingCard(ctx context.Context, userID string, deckID int, p PendingCard) (PendingCard, error) {
	f.lastUserID = userID
	if f.pendingErr != nil {
		return PendingCard{}, f.pendingErr
	}
	p.ID = 5
	p.DeckID = deckID
	f.addedPending = p
	return p, nil
}

func (f *fakeService) RemovePendingCard(ctx context.Context, userID string, deckID, id int) error {
	f.lastUserID = userID
	return f.removePendingErr
}

func (f *fakeService) GetAllDecks(ctx context.Context, userID string, filter Filter) ([]Deck, int, error) {
	f.lastUserID = userID
	f.lastFilter = filter
	return f.decks, f.getAllTotal, f.getAllErr
}

func (f *fakeService) GetDeck(ctx context.Context, userID string, id int) (Deck, error) {
	f.lastUserID = userID
	if f.getDeckErr != nil {
		return Deck{}, f.getDeckErr
	}
	return f.getDeck, nil
}

func (f *fakeService) CreateDeck(ctx context.Context, userID string, d Deck) (Deck, error) {
	f.lastUserID = userID
	f.lastCreatedDeck = d
	if f.createErr != nil {
		return Deck{}, f.createErr
	}
	d.ID = 1
	return d, nil
}

func (f *fakeService) UpdateDeck(ctx context.Context, userID string, id int, req updateDeckRequest) (Deck, error) {
	f.lastUserID = userID
	f.lastUpdateRequest = req
	if f.updateErr != nil {
		return Deck{}, f.updateErr
	}
	return f.updateDeck, nil
}

func (f *fakeService) DeleteDeck(ctx context.Context, userID string, id int) error {
	f.lastUserID = userID
	return f.deleteErr
}

func (f *fakeService) GetDeckCards(ctx context.Context, userID string, id int, sortField string, sortDesc bool) ([]DeckCard, error) {
	f.lastUserID = userID
	f.lastDeckCardsSort = sortField
	f.lastDeckCardsSortDesc = sortDesc
	if f.getDeckCardsErr != nil {
		return nil, f.getDeckCardsErr
	}
	return f.getDeckCards, nil
}

func (f *fakeService) PutCardInDeck(ctx context.Context, userID string, deckID, cardID int) error {
	f.lastUserID = userID
	return f.putCardErr
}

func (f *fakeService) RemoveCardFromDeck(ctx context.Context, userID string, deckID, cardID int) error {
	f.lastUserID = userID
	return f.removeCardErr
}

func setupRouter(service deckService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Next()
	})
	NewHandler(service).RegisterRoutes(router)
	return router
}

func TestHandler_GetDecks_PassesUserIDToService(t *testing.T) {
	service := &fakeService{
		decks:       []Deck{{ID: 1, Name: "Otterly Playful", Format: "commander"}},
		getAllTotal: 1,
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, testUserID, service.lastUserID)

	var response paginatedDecksResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response.Data, 1)
	assert.Equal(t, "Otterly Playful", response.Data[0].Name)
}

func TestHandler_GetDecks_PassesFormatFilterToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck?format=commander", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "commander", service.lastFilter.Format)
}

func TestHandler_GetDecks_DefaultsPageAndLimitWhenNotProvided(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, defaultPage, service.lastFilter.Page)
	assert.Equal(t, defaultLimit, service.lastFilter.Limit)
}

func TestHandler_GetDecks_PassesPageAndLimitToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck?page=2&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 2, service.lastFilter.Page)
	assert.Equal(t, 10, service.lastFilter.Limit)
}

func TestHandler_GetDecks_ReturnsBadRequestOnInvalidPage(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck?page=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetDecks_ReturnsBadRequestOnLimitAboveMax(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck?limit=101", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetDecks_DefaultsSortToUpdatedDescending(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "updated", service.lastFilter.SortField)
	assert.True(t, service.lastFilter.SortDesc)
}

func TestHandler_GetDecks_PassesSortToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck?sort=-name", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "name", service.lastFilter.SortField)
	assert.True(t, service.lastFilter.SortDesc)
}

func TestHandler_GetDecks_PassesAscendingSortToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck?sort=added", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "added", service.lastFilter.SortField)
	assert.False(t, service.lastFilter.SortDesc)
}

func TestHandler_GetDecks_ReturnsBadRequestOnInvalidSort(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck?sort=price", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetDecks_ReturnsPaginationEnvelope(t *testing.T) {
	service := &fakeService{
		decks:       []Deck{{ID: 1, Name: "Otterly Playful", Format: "commander"}},
		getAllTotal: 1,
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response paginatedDecksResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, defaultPage, response.Page)
	assert.Equal(t, defaultLimit, response.Limit)
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, 1, response.TotalPages)
}

func TestHandler_GetDecks_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{getAllErr: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_GetDeck_ReturnsDeckAsJSON(t *testing.T) {
	service := &fakeService{getDeck: Deck{ID: 1, Name: "Otterly Playful", Format: "commander"}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response deckResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Otterly Playful", response.Name)
}

func TestHandler_GetDeck_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetDeck_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{getDeckErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetDeck_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{getDeckErr: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_CreateDeck_ReturnsCreatedDeck(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": "Otterly Playful", "format": "commander"}`

	req := httptest.NewRequest(http.MethodPost, "/deck", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, testUserID, service.lastUserID)

	var response deckResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.ID)
	assert.Equal(t, "Otterly Playful", response.Name)
}

func TestHandler_CreateDeck_ReturnsBadRequestOnMissingRequiredField(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"format": "commander"}`

	req := httptest.NewRequest(http.MethodPost, "/deck", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateDeck_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{createErr: errors.New("insert failed")}
	router := setupRouter(service)

	body := `{"name": "Otterly Playful", "format": "commander"}`

	req := httptest.NewRequest(http.MethodPost, "/deck", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_CreateDeck_ReturnsBadRequestWhenCommanderDoesNotExist(t *testing.T) {
	service := &fakeService{createErr: ErrCommanderNotFound}
	router := setupRouter(service)

	body := `{"name": "Otterly Playful", "format": "commander", "commander_id": 9999}`

	req := httptest.NewRequest(http.MethodPost, "/deck", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "commander_id does not reference an existing card", response["error"])
}

func TestHandler_UpdateDeck_ReturnsUpdatedDeck(t *testing.T) {
	service := &fakeService{updateDeck: Deck{ID: 1, Name: "Renamed", Format: "modern"}}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response deckResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", response.Name)
}

func TestHandler_UpdateDeck_ReturnsBackgroundScryfallID(t *testing.T) {
	background := "436d6a84-4cea-4ca7-94aa-9d08280652af"
	service := &fakeService{updateDeck: Deck{ID: 1, Name: "Deck", Format: "commander", BackgroundScryfallID: &background}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(`{"background_scryfall_id": "`+background+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response deckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotNil(t, response.BackgroundScryfallID)
	assert.Equal(t, background, *response.BackgroundScryfallID)
}

func TestHandler_UpdateDeck_RejectsInvalidBackgroundScryfallID(t *testing.T) {
	router := setupRouter(&fakeService{})

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(`{"background_scryfall_id": "not-a-uuid"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateDeck_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/deck/abc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateDeck_ReturnsBadRequestOnInvalidBody(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": 1}`

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateDeck_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{updateErr: ErrNotFound}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/deck/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_UpdateDeck_ReturnsBadRequestWhenCommanderDoesNotExist(t *testing.T) {
	service := &fakeService{updateErr: ErrCommanderNotFound}
	router := setupRouter(service)

	body := `{"name": "Renamed", "commander_id": 999}`

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateDeck_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{updateErr: errors.New("update failed")}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_DeleteDeck_ReturnsNoContent(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
}

func TestHandler_DeleteDeck_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_DeleteDeck_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{deleteErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_DeleteDeck_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{deleteErr: errors.New("delete failed")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_GetDeckCards_ReturnsCardsAsJSON(t *testing.T) {
	service := &fakeService{
		getDeckCards: []DeckCard{
			{ID: 1, Name: "Black Lotus", SetCode: "lea", Foil: false},
		},
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/1/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []deckCardResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response, 1)
	assert.Equal(t, "Black Lotus", response[0].Name)
}

func TestHandler_GetDeckCards_IncludesColorsAndType(t *testing.T) {
	colors, cardType := "R", "Instant"
	service := &fakeService{
		getDeckCards: []DeckCard{
			{ID: 1, Name: "Lightning Bolt", Colors: &colors, CardType: &cardType, ColorIdentity: &colors},
			{ID: 2, Name: "Unknown Card"},
		},
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/1/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 2)
	assert.Equal(t, "R", response[0]["colors"])
	assert.Equal(t, "Instant", response[0]["card_type"])
	assert.Equal(t, "R", response[0]["color_identity"])
	assert.Nil(t, response[1]["color_identity"])
	assert.Contains(t, response[1], "colors")
	assert.Nil(t, response[1]["colors"])
	assert.Nil(t, response[1]["card_type"])
}

func TestHandler_GetDeckCards_PassesManaValueSortToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/1/cards?sort=-mana_value", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "mana_value", service.lastDeckCardsSort)
	assert.True(t, service.lastDeckCardsSortDesc)
}

func TestHandler_GetDeckCards_ReturnsBadRequestOnInvalidSort(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/1/cards?sort=price", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetDeckCards_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/abc/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetDeckCards_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{getDeckCardsErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/999/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetDeckCards_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{getDeckCardsErr: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/1/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_PutCardInDeck_ReturnsNoContent(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/deck/1/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
}

func TestHandler_PutCardInDeck_ReturnsBadRequestOnInvalidDeckID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/deck/abc/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_PutCardInDeck_ReturnsBadRequestOnInvalidCardID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/deck/1/cards/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_PutCardInDeck_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{putCardErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/deck/999/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "deck not found", response["error"])
}

func TestHandler_PutCardInDeck_ReturnsNotFoundWhenCardDoesNotExist(t *testing.T) {
	service := &fakeService{putCardErr: ErrCardNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/deck/1/cards/9999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "card not found", response["error"])
}

func TestHandler_PutCardInDeck_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{putCardErr: errors.New("insert failed")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/deck/1/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_RemoveCardFromDeck_ReturnsNoContent(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/1/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
}

func TestHandler_RemoveCardFromDeck_ReturnsBadRequestOnInvalidDeckID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/abc/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_RemoveCardFromDeck_ReturnsBadRequestOnInvalidCardID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/1/cards/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_RemoveCardFromDeck_ReturnsNotFoundWhenDeckDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{removeCardErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/999/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_RemoveCardFromDeck_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{removeCardErr: errors.New("delete failed")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/1/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_GetPendingCards_ReturnsTheList(t *testing.T) {
	service := &fakeService{pending: []PendingCard{{ID: 1, DeckID: 2, Name: "Sol Ring", Quantity: 2}}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/2/pending", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response []pendingCardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1)
	assert.Equal(t, "Sol Ring", response[0].Name)
	assert.Equal(t, 2, response[0].Quantity)
}

func TestHandler_GetPendingCards_ReturnsNotFoundForAnotherUsersDeck(t *testing.T) {
	router := setupRouter(&fakeService{pendingErr: ErrNotFound})

	req := httptest.NewRequest(http.MethodGet, "/deck/2/pending", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_AddPendingCard_NormalizesAndDefaultsQuantity(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": "Lightning Helix", "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", "set_code": "rav", "collector_number": "213", "colors": "rw", "color_identity": "rw", "card_type": "Instant", "mana_value": 2}`
	req := httptest.NewRequest(http.MethodPost, "/deck/2/pending", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, 1, service.addedPending.Quantity)
	assert.Equal(t, 2, service.addedPending.DeckID)
	assert.Equal(t, "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", service.addedPending.ScryfallID)
	require.NotNil(t, service.addedPending.Colors)
	assert.Equal(t, "WR", *service.addedPending.Colors)
}

func TestHandler_AddPendingCard_RejectsInvalidBodies(t *testing.T) {
	bodies := []string{
		`{"scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", "set_code": "rav", "collector_number": "213"}`,
		`{"name": "X", "scryfall_id": "nope", "set_code": "rav", "collector_number": "213"}`,
		`{"name": "X", "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", "set_code": "rav", "collector_number": "213", "quantity": 0}`,
		`{"name": "X", "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", "set_code": "rav", "collector_number": "213", "colors": "WX"}`,
		`{"name": "X", "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", "set_code": "rav", "collector_number": "213", "card_type": "Tribal"}`,
	}
	for _, body := range bodies {
		router := setupRouter(&fakeService{})
		req := httptest.NewRequest(http.MethodPost, "/deck/2/pending", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
	}
}

func TestHandler_RemovePendingCard(t *testing.T) {
	cases := map[error]int{nil: http.StatusNoContent, ErrPendingCardNotFound: http.StatusNotFound, ErrNotFound: http.StatusNotFound}
	for removeErr, expected := range cases {
		router := setupRouter(&fakeService{removePendingErr: removeErr})
		req := httptest.NewRequest(http.MethodDelete, "/deck/2/pending/4", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, expected, w.Code)
	}
}

func TestHandler_GetDeck_ReturnsPendingCount(t *testing.T) {
	router := setupRouter(&fakeService{getDeck: Deck{ID: 1, Name: "Deck", Format: "commander", CardCount: 98, PendingCount: 2}})

	req := httptest.NewRequest(http.MethodGet, "/deck/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response deckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 98, response.CardCount)
	assert.Equal(t, 2, response.PendingCount)
}

func TestHandler_UpdateDeck_AcceptsAPendingCommander(t *testing.T) {
	pendingID := 7
	service := &fakeService{updateDeck: Deck{ID: 1, Name: "Deck", Format: "commander", CommanderPendingID: &pendingID}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(`{"commander_pending_id": 7}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response deckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotNil(t, response.CommanderPendingID)
	assert.Equal(t, 7, *response.CommanderPendingID)
}

func postDeck(router *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/deck", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHandler_CreateDeck_IsUnlistedByDefault(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := postDeck(router, `{"name": "Otterly Playful", "format": "commander"}`)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, VisibilityUnlisted, service.lastCreatedDeck.Visibility)
	var response deckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "unlisted", response.Visibility)
}

func TestHandler_CreateDeck_AcceptsAVisibility(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := postDeck(router, `{"name": "Otterly Playful", "format": "commander", "visibility": "private"}`)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, VisibilityPrivate, service.lastCreatedDeck.Visibility)
}

func TestHandler_CreateDeck_RejectsAnUnknownVisibility(t *testing.T) {
	router := setupRouter(&fakeService{})

	w := postDeck(router, `{"name": "Otterly Playful", "format": "commander", "visibility": "friends"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateDeck_PassesTheVisibility(t *testing.T) {
	service := &fakeService{updateDeck: Deck{ID: 1, Name: "Deck", Format: "commander", Visibility: VisibilityPublic}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(`{"visibility": "public"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastUpdateRequest.Visibility)
	assert.Equal(t, "public", *service.lastUpdateRequest.Visibility)
	var response deckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "public", response.Visibility)
}

func TestHandler_UpdateDeck_RejectsAnUnknownVisibility(t *testing.T) {
	router := setupRouter(&fakeService{})

	req := httptest.NewRequest(http.MethodPatch, "/deck/1", bytes.NewBufferString(`{"visibility": "everyone"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateDeckRequest_ChangesTheVisibilityOnlyWhenGiven(t *testing.T) {
	d := Deck{Name: "Deck", Visibility: VisibilityUnlisted}

	name := "Renamed"
	assert.Equal(t, VisibilityUnlisted, updateDeckRequest{Name: &name}.applyTo(d).Visibility)

	public := VisibilityPublic
	assert.Equal(t, VisibilityPublic, updateDeckRequest{Visibility: &public}.applyTo(d).Visibility)
}
