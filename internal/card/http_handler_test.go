package card

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeService struct {
	cards      []Card
	total      int
	getAllErr  error
	lastUserID string
	lastFilter CardFilter

	getCard    Card
	getCardErr error

	createErr error

	updateCard        Card
	lastUpdateRequest updateCardRequest
	updateErr         error

	deleteErr error

	deleteAllCount  int
	deleteAllErr    error
	deleteAllCalled bool
}

func (f *fakeService) GetAllCards(ctx context.Context, userID string, filter CardFilter) ([]Card, int, error) {
	f.lastUserID = userID
	f.lastFilter = filter
	return f.cards, f.total, f.getAllErr
}

func (f *fakeService) GetCard(ctx context.Context, userID string, id int) (Card, error) {
	f.lastUserID = userID
	if f.getCardErr != nil {
		return Card{}, f.getCardErr
	}
	return f.getCard, nil
}

func (f *fakeService) CreateCard(ctx context.Context, userID string, c Card) (Card, error) {
	f.lastUserID = userID
	if f.createErr != nil {
		return Card{}, f.createErr
	}
	c.ID = 1
	return c, nil
}

func (f *fakeService) UpdateCard(ctx context.Context, userID string, id int, req updateCardRequest) (Card, error) {
	f.lastUserID = userID
	f.lastUpdateRequest = req
	if f.updateErr != nil {
		return Card{}, f.updateErr
	}
	return f.updateCard, nil
}

func (f *fakeService) DeleteAllCards(ctx context.Context, userID string) (int, error) {
	f.lastUserID = userID
	f.deleteAllCalled = true
	return f.deleteAllCount, f.deleteAllErr
}

func (f *fakeService) DeleteCard(ctx context.Context, userID string, id int) error {
	f.lastUserID = userID
	return f.deleteErr
}

func setupRouter(service cardService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Next()
	})
	NewHandler(service).RegisterRoutes(router)
	return router
}

func TestHandler_GetCards_ReturnsPaginatedCardsAsJSON(t *testing.T) {
	service := &fakeService{
		cards: []Card{
			{ID: 1, Name: "Black Lotus", SetCode: "lea", Foil: false},
		},
		total: 1,
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, testUserID, service.lastUserID)

	var response paginatedCardsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response.Data, 1)
	assert.Equal(t, "Black Lotus", response.Data[0].Name)
}

func TestHandler_GetCards_UsesDefaultPageAndLimitWhenAbsent(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, defaultPage, service.lastFilter.Page)
	assert.Equal(t, defaultLimit, service.lastFilter.Limit)
}

func TestHandler_GetCards_PassesPageLimitAndNameToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?page=3&limit=10&name=bolt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 3, service.lastFilter.Page)
	assert.Equal(t, 10, service.lastFilter.Limit)
	assert.Equal(t, "bolt", service.lastFilter.Name)
}

func TestHandler_GetCards_PassesSortToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?sort=-name", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "name", service.lastFilter.SortField)
	assert.True(t, service.lastFilter.SortDesc)
}

func TestHandler_GetCards_PassesTheGroupingToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?group=mana&sort=-name", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "mana", service.lastFilter.GroupBy)
	assert.Equal(t, "name", service.lastFilter.SortField)
	assert.True(t, service.lastFilter.SortDesc)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/cards?group=storage", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCards_PassesManaValueSortToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?sort=-mana_value", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "mana_value", service.lastFilter.SortField)
	assert.True(t, service.lastFilter.SortDesc)
}

func TestHandler_GetCards_ReturnsStacksWithQuantityAndCopyIDs(t *testing.T) {
	service := &fakeService{
		cards: []Card{{ID: 4, Name: "Lightning Bolt", SetCode: "2xm", CopyIDs: []int{4, 7, 9}}},
		total: 1,
	}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?stack=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, service.lastFilter.Stack)

	var response paginatedCardsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	assert.Equal(t, 3, response.Data[0].Quantity)
	assert.Equal(t, []int{4, 7, 9}, response.Data[0].CopyIDs)
}

func TestHandler_GetCards_OmitsQuantityWhenNotStacked(t *testing.T) {
	service := &fakeService{cards: []Card{{ID: 1, Name: "Black Lotus"}}, total: 1}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, service.lastFilter.Stack)
	assert.NotContains(t, w.Body.String(), "quantity")
	assert.NotContains(t, w.Body.String(), "copy_ids")
}

func TestHandler_GetCards_ReturnsBadRequestOnInvalidStack(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?stack=maybe", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCards_ReturnsBadRequestOnInvalidSort(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?sort=price", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCards_ReturnsBadRequestOnInvalidPage(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?page=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCards_ReturnsBadRequestOnInvalidLimit(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?limit=101", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCards_PassesStorageIDQueryParamToService(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?storage_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastFilter.StorageID)
	assert.Equal(t, 1, *service.lastFilter.StorageID)
}

func TestHandler_GetCards_PassesColorIdentityFilter(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?color_identity=gw", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastFilter.ColorIdentity)
	assert.Equal(t, "WG", *service.lastFilter.ColorIdentity)
}

func TestHandler_GetCards_AcceptsEmptyColorIdentityForColorlessCommanders(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?color_identity=", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastFilter.ColorIdentity)
	assert.Equal(t, "", *service.lastFilter.ColorIdentity)
}

func TestHandler_GetCards_DoesNotFilterOnColorIdentityByDefault(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, service.lastFilter.ColorIdentity)
}

func TestHandler_GetCards_RejectsInvalidColorIdentity(t *testing.T) {
	router := setupRouter(&fakeService{})

	req := httptest.NewRequest(http.MethodGet, "/cards?color_identity=WX", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCards_ReturnsErrorInvalidStorageID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards?storage_id=invalid_storage_id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCards_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{getAllErr: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_GetCard_ReturnsCardAsJSON(t *testing.T) {
	service := &fakeService{getCard: Card{ID: 1, Name: "Black Lotus", SetCode: "lea", Foil: false}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response cardResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Black Lotus", response.Name)
}

func TestHandler_GetCard_ReturnsNotFoundWhenCardDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{getCardErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetCard_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCard_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{getCardErr: errors.New("database unreachable")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/cards/1", nil)
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
		"collector_number": "267",
		"foil": false,
		"storage_id": 1
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, testUserID, service.lastUserID)

	var response cardResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.ID)
	assert.Equal(t, "Counterspell", response.Name)
}

func TestHandler_CreateCard_AllowsNilStorageID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{
		"name": "Counterspell",
		"scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
		"set_code": "mh2",
		"collector_number": "267",
		"foil": false
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var response cardResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Nil(t, response.StorageID)
}

func TestHandler_CreateCard_ReturnsBadRequestOnMissingRequiredField(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{
		"scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
		"set_code": "mh2",
		"collector_number": "267",
		"foil": false,
		"storage_id": 1
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
		"collector_number": "267",
		"foil": false,
		"storage_id": 1
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
		"collector_number": "267",
		"foil": false,
		"storage_id": 1
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_CreateCard_ReturnsBadRequestWhenStorageDoesNotExist(t *testing.T) {
	service := &fakeService{createErr: ErrStorageNotFound}
	router := setupRouter(service)

	body := `{
		"name": "Counterspell",
		"scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f",
		"set_code": "mh2",
		"collector_number": "267",
		"foil": false,
		"storage_id": 9999
	}`

	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "storage_id does not reference an existing storage", response["error"])
}

func TestHandler_UpdateCard_ReturnsUpdatedCard(t *testing.T) {
	service := &fakeService{updateCard: Card{ID: 1, Name: "Renamed", SetCode: "lea", Foil: false}}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/cards/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response cardResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", response.Name)
}

func patchCard(router *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/cards/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHandler_UpdateCard_NullStorageIDRemovesCardFromStorage(t *testing.T) {
	service := &fakeService{updateCard: Card{ID: 1, Name: "Black Lotus"}}
	router := setupRouter(service)

	w := patchCard(router, `{"storage_id": null}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, service.lastUpdateRequest.StorageID.Set)
	assert.Nil(t, service.lastUpdateRequest.StorageID.Value)
}

func TestHandler_UpdateCard_AbsentStorageIDLeavesStorageUnchanged(t *testing.T) {
	service := &fakeService{updateCard: Card{ID: 1, Name: "Renamed"}}
	router := setupRouter(service)

	w := patchCard(router, `{"name": "Renamed"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, service.lastUpdateRequest.StorageID.Set)
}

func TestHandler_UpdateCard_SetsStorageID(t *testing.T) {
	service := &fakeService{updateCard: Card{ID: 1, Name: "Black Lotus"}}
	router := setupRouter(service)

	w := patchCard(router, `{"storage_id": 7}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastUpdateRequest.StorageID.Value)
	assert.Equal(t, 7, *service.lastUpdateRequest.StorageID.Value)
}

func TestHandler_UpdateCard_ReturnsBadRequestOnNonPositiveStorageID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := patchCard(router, `{"storage_id": 0}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateCard_ReturnsBadRequestOnNonIntegerStorageID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := patchCard(router, `{"storage_id": "seven"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateCard_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/cards/abc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateCard_ReturnsBadRequestOnInvalidBody(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": 1}`

	req := httptest.NewRequest(http.MethodPatch, "/cards/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateCard_ReturnsNotFoundWhenCardDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{updateErr: ErrNotFound}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/cards/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_UpdateCard_ReturnsBadRequestWhenStorageDoesNotExist(t *testing.T) {
	service := &fakeService{updateErr: ErrStorageNotFound}
	router := setupRouter(service)

	body := `{"name": "Renamed", "storage_id": 999}`

	req := httptest.NewRequest(http.MethodPatch, "/cards/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateCard_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{updateErr: errors.New("update failed")}
	router := setupRouter(service)

	body := `{"name": "Renamed"}`

	req := httptest.NewRequest(http.MethodPatch, "/cards/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_DeleteCard_ReturnsNoContent(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/cards/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
}

func TestHandler_DeleteCard_ReturnsBadRequestOnInvalidID(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/cards/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_DeleteCard_ReturnsNotFoundWhenCardDoesNotBelongToUser(t *testing.T) {
	service := &fakeService{deleteErr: ErrNotFound}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/cards/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_DeleteCard_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{deleteErr: errors.New("delete failed")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/cards/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_DeleteAllCards_ReturnsDeletedCount(t *testing.T) {
	service := &fakeService{deleteAllCount: 42}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/cards?confirm=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response deleteAllCardsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 42, response.Deleted)
	assert.Equal(t, testUserID, service.lastUserID)
}

func TestHandler_DeleteAllCards_RequiresConfirmation(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/cards", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, service.deleteAllCalled)
}

func TestHandler_DeleteAllCards_ReturnsErrorOnServiceFailure(t *testing.T) {
	service := &fakeService{deleteAllErr: errors.New("delete failed")}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/cards?confirm=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_GetCards_AcceptsColorAndTypeSorts(t *testing.T) {
	for _, sort := range []string{"color", "-color", "type", "-type"} {
		service := &fakeService{}
		router := setupRouter(service)

		req := httptest.NewRequest(http.MethodGet, "/cards?sort="+sort, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, sort)
		assert.Equal(t, strings.TrimPrefix(sort, "-"), service.lastFilter.SortField)
		assert.Equal(t, strings.HasPrefix(sort, "-"), service.lastFilter.SortDesc)
	}
}

func postCard(router *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

const cardBodyPrefix = `"name": "Lightning Helix", "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", "set_code": "rav", "collector_number": "213"`

func TestHandler_CreateCard_NormalizesColorsAndKeepsType(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := postCard(router, `{`+cardBodyPrefix+`, "colors": "rw", "card_type": "Instant"}`)

	require.Equal(t, http.StatusCreated, w.Code)
	var response cardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotNil(t, response.Colors)
	assert.Equal(t, "WR", *response.Colors)
	require.NotNil(t, response.CardType)
	assert.Equal(t, "Instant", *response.CardType)
}

func TestHandler_CreateCard_NormalizesColorIdentity(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := postCard(router, `{`+cardBodyPrefix+`, "color_identity": "gub"}`)

	require.Equal(t, http.StatusCreated, w.Code)
	var response cardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotNil(t, response.ColorIdentity)
	assert.Equal(t, "UBG", *response.ColorIdentity)
}

func TestHandler_CreateCard_RejectsInvalidColorIdentity(t *testing.T) {
	router := setupRouter(&fakeService{})

	w := postCard(router, `{`+cardBodyPrefix+`, "color_identity": "C"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateCard_AcceptsEmptyColorsForColorlessCards(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := postCard(router, `{`+cardBodyPrefix+`, "colors": "", "card_type": "Artifact"}`)

	require.Equal(t, http.StatusCreated, w.Code)
	var response cardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotNil(t, response.Colors)
	assert.Equal(t, "", *response.Colors)
}

func TestHandler_CreateCard_LeavesDetailsNullWhenAbsent(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := postCard(router, `{`+cardBodyPrefix+`}`)

	require.Equal(t, http.StatusCreated, w.Code)
	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Nil(t, response["colors"])
	assert.Nil(t, response["card_type"])
}

func TestHandler_CreateCard_RejectsInvalidColors(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := postCard(router, `{`+cardBodyPrefix+`, "colors": "WX"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateCard_RejectsInvalidCardType(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := postCard(router, `{`+cardBodyPrefix+`, "card_type": "creature"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateCard_RejectsInvalidColors(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := patchCard(router, `{"colors": "purple"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CreateCard_StoresAndReturnsProxy(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"name": "Mana Crypt", "scryfall_id": "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", "set_code": "2xm", "collector_number": "270", "proxy": true}`
	req := httptest.NewRequest(http.MethodPost, "/cards", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var response cardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Proxy)
}

func TestHandler_UpdateCard_SetsProxy(t *testing.T) {
	service := &fakeService{updateCard: Card{ID: 1, Name: "Mana Crypt", Proxy: true}}
	router := setupRouter(service)

	w := patchCard(router, `{"proxy": true}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastUpdateRequest.Proxy)
	assert.True(t, *service.lastUpdateRequest.Proxy)
	var response cardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Proxy)
}
