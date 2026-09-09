package storage

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
	storages  []Storage
	getAllErr error

	createErr error
}

func (f *fakeService) GetAllStorages(ctx context.Context) ([]Storage, error) {
	return f.storages, f.getAllErr
}

func (f *fakeService) CreateStorage(ctx context.Context, storage Storage) (Storage, error) {
	if f.createErr != nil {
		return Storage{}, f.createErr
	}
	storage.ID = 1
	return storage, nil
}

func setupRouter(service storageService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	return router
}

func TestHandler_GetStorages_ReturnsStoragesAsJSON(t *testing.T) {
	service := &fakeService{
		storages: []Storage{
			{ID: 1, Name: "Vintage Collection", Type: "binder"},
		},
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []storageResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response, 1)
	assert.Equal(t, "Vintage Collection", response[0].Name)
	assert.Equal(t, "binder", response[0].Type)
}

func TestHandler_GetStorages_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{getAllErr: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_CreateStorage_ReturnsCreatedStorage(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{
		"name": "Vintage Collection",
		"type": "binder"
	}`

	req := httptest.NewRequest(http.MethodPost, "/storage", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var response storageResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.ID)
	assert.Equal(t, "Vintage Collection", response.Name)
	assert.Equal(t, "binder", response.Type)
}

func TestHandler_CreateStorage_ReturnsBadRequestOnMissingRequiredField(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	// missing "name"
	body := `{
		"type": "binder"
	}`

	req := httptest.NewRequest(http.MethodPost, "/storage", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateStorage_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{createErr: errors.New("insert failed")}
	router := setupRouter(service)

	body := `{
		"name": "Vintage Collection",
		"type": "binder"
	}`

	req := httptest.NewRequest(http.MethodPost, "/storage", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
