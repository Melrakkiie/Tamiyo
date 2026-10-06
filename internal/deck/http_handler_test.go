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

	updateDeck Deck
	updateErr  error

	deleteErr error

	getDeckCards          []DeckCard
	getDeckCardsErr       error
	lastDeckCardsSort     string
	lastDeckCardsSortDesc bool

	putCardErr    error
	removeCardErr error
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
	if f.createErr != nil {
		return Deck{}, f.createErr
	}
	d.ID = 1
	return d, nil
}

func (f *fakeService) UpdateDeck(ctx context.Context, userID string, id int, req updateDeckRequest) (Deck, error) {
	f.lastUserID = userID
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
			{ID: 1, Name: "Lightning Bolt", Colors: &colors, CardType: &cardType},
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
