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
