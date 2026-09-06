package card

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
	cards     []Card
	getAllErr error

	createdCard Card
	createErr   error
}

func (f *fakeService) GetAllCards(ctx context.Context) ([]Card, error) {
	return f.cards, f.getAllErr
}

func (f *fakeService) CreateCard(ctx context.Context, c Card) (Card, error) {
	if f.createErr != nil {
		return Card{}, f.createErr
	}
	c.ID = 1
	return c, nil
}

func setupRouter(service cardService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	return router
}

func TestHandler_GetCards_ReturnsCardsAsJSON(t *testing.T) {
	service := &fakeService{
		cards: []Card{
			{ID: 1, Name: "Black Lotus", SetCode: "lea", Foil: false},
		},
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []cardResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response, 1)
	assert.Equal(t, "Black Lotus", response[0].Name)
	assert.Equal(t, "lea", response[0].SetCode)
}

func TestHandler_GetCards_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{getAllErr: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_CreateCard_ReturnsCreatedCard(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{
		"name": "Counterspell",
		"scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
		"set_code": "mh2",
		"collector_number": 267,
		"foil": false,
		"binder_name": "Blue Control",
		"binder_type": "deckbox"
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var response cardResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.ID)
	assert.Equal(t, "Counterspell", response.Name)
	assert.Equal(t, "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", response.ScryfallID)
	assert.Equal(t, "mh2", response.SetCode)
	assert.Equal(t, 267, response.CollectorNumber)
	assert.Equal(t, false, response.Foil)
	assert.Equal(t, "Blue Control", response.BinderName)
	assert.Equal(t, "deckbox", response.BinderType)
}

func TestHandler_CreateCard_ReturnsBadRequestOnMissingRequiredField(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	// missing "name"
	body := `{
		"scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
		"set_code": "mh2",
		"collector_number": 267,
		"foil": false,
		"binder_name": "Blue Control",
		"binder_type": "deckbox"
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateCard_ReturnsBadRequestOnInvalidScryfallID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{
		"name": "Counterspell",
		"scryfall_id": "not-a-valid-uuid",
		"set_code": "mh2",
		"collector_number": 267,
		"foil": false,
		"binder_name": "Blue Control",
		"binder_type": "deckbox"
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateCard_ReturnsBadRequestOnInvalidCollectorNumber(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{
		"name": "Counterspell",
		"scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
		"set_code": "mh2",
		"collector_number": 0,
		"foil": false,
		"binder_name": "Blue Control",
		"binder_type": "deckbox"
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateCard_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{createErr: errors.New("insert failed")}
	router := setupRouter(service)

	body := `{
		"name": "Counterspell",
		"scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
		"set_code": "mh2",
		"collector_number": 267,
		"foil": false,
		"binder_name": "Blue Control",
		"binder_type": "deckbox"
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
