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
	decks     []Deck
	getAllErr error

	getDeck    Deck
	getDeckErr error

	createErr error

	updateDeck Deck
	updateErr  error

	deleteErr error

	getDeckCards    []DeckCard
	getDeckCardsErr error

	putCardErr error

	removeCardErr error
}

func (f *fakeService) GetAllDecks(ctx context.Context) ([]Deck, error) {
	return f.decks, f.getAllErr
}

func (f *fakeService) GetDeck(ctx context.Context, id int) (Deck, error) {
	if f.getDeckErr != nil {
		return Deck{}, f.getDeckErr
	}
	return f.getDeck, nil
}

func (f *fakeService) CreateDeck(ctx context.Context, d Deck) (Deck, error) {
	if f.createErr != nil {
		return Deck{}, f.createErr
	}
	d.ID = 1
	return d, nil
}

func (f *fakeService) UpdateDeck(ctx context.Context, id int, req updateDeckRequest) (Deck, error) {
	if f.updateErr != nil {
		return Deck{}, f.updateErr
	}
	return f.updateDeck, nil
}

func (f *fakeService) DeleteDeck(ctx context.Context, id int) error {
	return f.deleteErr
}

func (f *fakeService) GetDeckCards(ctx context.Context, id int) ([]DeckCard, error) {
	if f.getDeckCardsErr != nil {
		return nil, f.getDeckCardsErr
	}
	return f.getDeckCards, nil
}

func (f *fakeService) PutCardInDeck(ctx context.Context, deckID, cardID int) error {
	return f.putCardErr
}

func (f *fakeService) RemoveCardFromDeck(ctx context.Context, deckID, cardID int) error {
	return f.removeCardErr
}

func setupRouter(service deckService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	return router
}

func TestHandler_GetDecks_ReturnsDecksAsJSON(t *testing.T) {
	service := &fakeService{
		decks: []Deck{
			{ID: 1, Name: "Otterly Playful", Format: "commander"},
		},
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []deckResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response, 1)
	assert.Equal(t, "Otterly Playful", response[0].Name)
	assert.Equal(t, "commander", response[0].Format)
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

	require.Equal(t, http.StatusCreated, w.Code)

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

func TestHandler_GetDeck_ReturnsNotFoundWhenDeckDoesNotExist(t *testing.T) {
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

	body := `{
		"name": "Otterly Playful",
		"format": "commander"
	}`

	req := httptest.NewRequest(http.MethodPost, "/deck", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var response deckResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.ID)
	assert.Equal(t, "Otterly Playful", response.Name)
	assert.Equal(t, "commander", response.Format)
}

func TestHandler_CreateDeck_ReturnsBadRequestOnMissingRequiredField(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	// missing "name"
	body := `{
		"format": "commander"
	}`

	req := httptest.NewRequest(http.MethodPost, "/deck", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateDeck_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{createErr: errors.New("insert failed")}
	router := setupRouter(service)

	body := `{
		"name": "Otterly Playful",
		"format": "commander"
	}`

	req := httptest.NewRequest(http.MethodPost, "/deck", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_CreateDeck_ReturnsBadRequestWhenCommanderDoesNotExist(t *testing.T) {
	service := &fakeService{createErr: ErrCommanderNotFound}
	router := setupRouter(service)

	body := `{
		"name": "Otterly Playful",
		"format": "commander",
		"storage_id": 9999
	}`

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
	service := &fakeService{updateDeck: Deck{ID: 1, Name: "Renamed", Format: "modern", CommanderID: nil}}
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

func TestHandler_UpdateDeck_ReturnsNotFoundWhenDeckDoesNotExist(t *testing.T) {
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

func TestHandler_DeleteDeck_ReturnsNotFoundWhenDeckDoesNotExist(t *testing.T) {
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
	assert.Equal(t, "lea", response[0].SetCode)
}

func TestHandler_GetDeckCards_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/abc/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetDeckCards_ReturnsNotFoundWhenDeckDoesNotExist(t *testing.T) {
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

func TestHandler_PutCardInDeck_ReturnsNotFoundWhenDeckDoesNotExist(t *testing.T) {
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

func TestHandler_RemoveCardFromDeck_ReturnsNoContentEvenWhenLinkDoesNotExist(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/1/cards/9999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandler_RemoveCardFromDeck_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{removeCardErr: errors.New("delete failed")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/deck/1/cards/4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
