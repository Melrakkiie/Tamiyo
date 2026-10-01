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
	storages    []Storage
	getAllTotal int
	getAllErr   error
	lastUserID  string
	lastFilter  Filter

	getStorage    Storage
	getStorageErr error

	createErr error

	updateStorage Storage
	updateErr     error

	deleteErr error
}

func (f *fakeService) GetAllStorages(ctx context.Context, userID string, filter Filter) ([]Storage, int, error) {
	f.lastUserID = userID
	f.lastFilter = filter
	return f.storages, f.getAllTotal, f.getAllErr
}

func (f *fakeService) GetStorage(ctx context.Context, userID string, id int) (Storage, error) {
	f.lastUserID = userID
	if f.getStorageErr != nil {
		return Storage{}, f.getStorageErr
	}
	return f.getStorage, nil
}

func (f *fakeService) CreateStorage(ctx context.Context, userID string, storage Storage) (Storage, error) {
	f.lastUserID = userID
	if f.createErr != nil {
		return Storage{}, f.createErr
	}
	storage.ID = 1
	return storage, nil
}

func (f *fakeService) UpdateStorage(ctx context.Context, userID string, id int, req updateStorageRequest) (Storage, error) {
	f.lastUserID = userID
	if f.updateErr != nil {
		return Storage{}, f.updateErr
	}
	return f.updateStorage, nil
}

func (f *fakeService) DeleteStorage(ctx context.Context, userID string, id int) error {
	f.lastUserID = userID
	return f.deleteErr
}

func setupRouter(service storageService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Next()
	})
	NewHandler(service).RegisterRoutes(router)
	return router
}

func TestHandler_GetStorages_PassesUserIDToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, testUserID, service.lastUserID)
}

func TestHandler_GetStorages_PassesTypeFilterToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage?type=binder", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "binder", service.lastFilter.Type)
}

func TestHandler_GetStorages_DefaultsPageAndLimitWhenNotProvided(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, defaultPage, service.lastFilter.Page)
	assert.Equal(t, defaultLimit, service.lastFilter.Limit)
}

func TestHandler_GetStorages_PassesPageAndLimitToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage?page=2&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 2, service.lastFilter.Page)
	assert.Equal(t, 10, service.lastFilter.Limit)
}

func TestHandler_GetStorages_ReturnsBadRequestOnInvalidPage(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage?page=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetStorages_ReturnsBadRequestOnLimitAboveMax(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage?limit=101", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetStorages_ReturnsPaginationEnvelope(t *testing.T) {
	service := &fakeService{
		storages:    []Storage{{ID: 1, Name: "Vintage Collection", Type: "binder"}},
		getAllTotal: 1,
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response paginatedStorageResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Len(t, response.Data, 1)
	assert.Equal(t, defaultPage, response.Page)
	assert.Equal(t, defaultLimit, response.Limit)
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, 1, response.TotalPages)
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

	require.Equal(t, http.StatusOK, w.Code)

	var response storageResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Vintage Collection", response.Name)
}

func TestHandler_GetStorage_ReturnsNotFoundWhenStorageDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{getStorageErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetStorage_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/storage/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateStorage_ReturnsCreatedStorage(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": "Vintage Collection", "type": "binder"}`
	req := httptest.NewRequest(http.MethodPost, "/storage", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, testUserID, service.lastUserID)
}

func TestHandler_CreateStorage_ReturnsBadRequestOnMissingRequiredField(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"type": "binder"}`
	req := httptest.NewRequest(http.MethodPost, "/storage", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
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
}

func TestHandler_UpdateStorage_ReturnsNotFoundWhenStorageDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{updateErr: ErrNotFound}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`
	req := httptest.NewRequest(http.MethodPatch, "/storage/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_DeleteStorage_ReturnsNoContent(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/storage/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandler_DeleteStorage_ReturnsNotFoundWhenStorageDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{deleteErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/storage/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
