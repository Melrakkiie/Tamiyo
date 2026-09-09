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

	getStorage    Storage
	getStorageErr error

	createErr error

	updateStorage Storage
	updateErr     error
}

func (f *fakeService) GetAllStorages(ctx context.Context) ([]Storage, error) {
	return f.storages, f.getAllErr
}

func (f *fakeService) GetStorage(ctx context.Context, id int) (Storage, error) {
	if f.getStorageErr != nil {
		return Storage{}, f.getStorageErr
	}
	return f.getStorage, nil
}

func (f *fakeService) CreateStorage(ctx context.Context, storage Storage) (Storage, error) {
	if f.createErr != nil {
		return Storage{}, f.createErr
	}
	storage.ID = 1
	return storage, nil
}

func (f *fakeService) UpdateStorage(ctx context.Context, id int, req updateStorageRequest) (Storage, error) {
	if f.updateErr != nil {
		return Storage{}, f.updateErr
	}
	return f.updateStorage, nil
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

func TestHandler_GetStorage_ReturnsStorageAsJSON(t *testing.T) {
	service := &fakeService{getStorage: Storage{ID: 1, Name: "Vintage Collection", Type: "binder"}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var response storageResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Vintage Collection", response.Name)
}

func TestHandler_GetStorage_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetStorage_ReturnsNotFoundWhenStorageDoesNotExist(t *testing.T) {
	service := &fakeService{getStorageErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetStorage_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{getStorageErr: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage/1", nil)
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

func TestHandler_UpdateStorage_ReturnsUpdatedStorage(t *testing.T) {
	service := &fakeService{updateStorage: Storage{ID: 1, Name: "Renamed", Type: "binder"}}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/storage/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response storageResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", response.Name)
}

func TestHandler_UpdateStorage_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/storage/abc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateStorage_ReturnsNotFoundWhenStorageDoesNotExist(t *testing.T) {
	service := &fakeService{updateErr: ErrNotFound}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/storage/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_UpdateStorage_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{updateErr: errors.New("update failed")}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/storage/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
