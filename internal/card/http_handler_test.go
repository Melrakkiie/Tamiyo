package card

import (
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
	cards []Card
	err   error
}

func (f *fakeService) GetAllCards(ctx context.Context) ([]Card, error) {
	return f.cards, f.err
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
	service := &fakeService{err: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
