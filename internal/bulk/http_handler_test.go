package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
)

type fakeImportService struct {
	summary Summary
	err     error

	lastUserID            string
	lastMoxfieldStorageID int
	lastManaBoxStorageID  *int
	lastListStorageID     *int
	lastExportStorageID   *int
	lastFileContent       string

	exportContent    string
	exportErr        error
	lastExportUser   string
	lastExportDeck   string
	lastExportFormat string

	refreshSummary DetailsRefreshSummary
	lastAfterID    int

	commitSummary PendingCommitSummary
	lastDeckID    string

	lastIntoDeckCommander bool
	lastTamiyoStorageID   *int
	lastExportTags        bool
	tamiyoImportCalled    bool
	lastQuantity          *int
	lastStorageID         *int
	lastPendingID         *int
	commitCalled          bool

	duplicateResult DuplicateSummary
	lastSharedTags  bool
	lastCollect     CollectRequest
}

func (f *fakeImportService) ExportSharedDeck(ctx context.Context, deckID string, format string, withTags bool, w io.Writer) error {
	f.lastExportDeck = deckID
	f.lastExportFormat = format
	f.lastSharedTags = withTags
	if f.exportErr != nil {
		return f.exportErr
	}
	_, err := io.WriteString(w, f.exportContent)
	return err
}

func (f *fakeImportService) DuplicateDeck(ctx context.Context, userID string, deckID string) (DuplicateSummary, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.duplicateResult, f.err
}

func (f *fakeImportService) CollectDeck(ctx context.Context, userID string, deckID string, req CollectRequest) (Summary, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	f.lastCollect = req
	return f.summary, f.err
}

func (f *fakeImportService) CommitPendingCards(ctx context.Context, userID string, deckID string, storageID, pendingID, quantity *int) (PendingCommitSummary, error) {
	f.lastQuantity = quantity
	f.lastUserID = userID
	f.lastDeckID = deckID
	f.lastStorageID = storageID
	f.lastPendingID = pendingID
	f.commitCalled = true
	return f.commitSummary, f.err
}

func (f *fakeImportService) ImportManaBox(ctx context.Context, userID string, storageID *int, r io.Reader) (Summary, error) {
	f.lastUserID = userID
	f.lastManaBoxStorageID = storageID
	f.readFile(r)
	return f.summary, f.err
}

func (f *fakeImportService) ImportMoxfieldCollection(ctx context.Context, userID string, storageID int, r io.Reader) (Summary, error) {
	f.lastUserID = userID
	f.lastMoxfieldStorageID = storageID
	f.readFile(r)
	return f.summary, f.err
}

func (f *fakeImportService) ImportCardList(ctx context.Context, userID string, storageID *int, r io.Reader) (Summary, error) {
	f.lastUserID = userID
	f.lastListStorageID = storageID
	f.readFile(r)
	return f.summary, f.err
}

func (f *fakeImportService) RefreshCardDetails(ctx context.Context, userID string, afterID int) (DetailsRefreshSummary, error) {
	f.lastUserID = userID
	f.lastAfterID = afterID
	return f.refreshSummary, f.err
}

func (f *fakeImportService) readFile(r io.Reader) {
	b, _ := io.ReadAll(r)
	f.lastFileContent = string(b)
}

func (f *fakeImportService) ExportManaBox(ctx context.Context, userID string, storageID *int, w io.Writer) error {
	f.lastExportUser = userID
	f.lastExportStorageID = storageID
	if f.exportErr != nil {
		return f.exportErr
	}
	_, err := w.Write([]byte(f.exportContent))
	return err
}

func (f *fakeImportService) ExportMoxfieldCollection(ctx context.Context, userID string, storageID *int, w io.Writer) error {
	f.lastExportUser = userID
	f.lastExportStorageID = storageID
	if f.exportErr != nil {
		return f.exportErr
	}
	_, err := w.Write([]byte(f.exportContent))
	return err
}

func (f *fakeImportService) ImportIntoDeck(ctx context.Context, userID string, deckID string, commanderFromFirstLine bool, r io.Reader) (Summary, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	f.lastIntoDeckCommander = commanderFromFirstLine
	content, _ := io.ReadAll(r)
	f.lastFileContent = string(content)
	return f.summary, f.err
}

func (f *fakeImportService) ImportTamiyoCollection(ctx context.Context, userID string, storageID *int, r io.Reader) (Summary, error) {
	f.lastUserID = userID
	f.lastTamiyoStorageID = storageID
	f.tamiyoImportCalled = true
	f.readFile(r)
	return f.summary, f.err
}

func (f *fakeImportService) ExportTamiyoCollection(ctx context.Context, userID string, storageID *int, w io.Writer) error {
	f.lastExportUser = userID
	f.lastExportStorageID = storageID
	if f.exportErr != nil {
		return f.exportErr
	}
	_, err := w.Write([]byte(f.exportContent))
	return err
}

func (f *fakeImportService) ExportTamiyoDeck(ctx context.Context, userID string, deckID string, withTags bool, w io.Writer) error {
	f.lastExportUser = userID
	f.lastExportDeck = deckID
	f.lastExportFormat = DeckExportTamiyo
	f.lastExportTags = withTags
	if f.exportErr != nil {
		return f.exportErr
	}
	_, err := w.Write([]byte(f.exportContent))
	return err
}

func (f *fakeImportService) ExportDeck(ctx context.Context, userID string, deckID string, format string, w io.Writer) error {
	f.lastExportUser = userID
	f.lastExportDeck = deckID
	f.lastExportFormat = format
	if f.exportErr != nil {
		return f.exportErr
	}
	_, err := w.Write([]byte(f.exportContent))
	return err
}

func setupRouter(service importService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Next()
	})
	NewHandler(service).RegisterRoutes(router)
	return router
}

func multipartRequest(t *testing.T, path, fileContent string, fields map[string]string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}

	if fileContent != "" {
		part, err := writer.CreateFormFile("file", "upload.csv")
		require.NoError(t, err)
		_, err = part.Write([]byte(fileContent))
		require.NoError(t, err)
	}

	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestImportManaBox_ReturnsSummaryOnSuccess(t *testing.T) {
	service := &fakeImportService{summary: Summary{CardsCreated: 3}}
	router := setupRouter(service)

	req := multipartRequest(t, "/import/manabox", "some,csv,content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got Summary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 3, got.CardsCreated)
	assert.Equal(t, testUserID, service.lastUserID)
	assert.Equal(t, "some,csv,content", service.lastFileContent)
}

func TestImportManaBox_RequiresFile(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := multipartRequest(t, "/import/manabox", "", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportManaBox_InvalidFileReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeImportService{err: ErrInvalidFile})

	req := multipartRequest(t, "/import/manabox", "garbage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportManaBox_StorageIDIsOptional(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := multipartRequest(t, "/import/manabox", "csv content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, service.lastManaBoxStorageID)
}

func TestImportManaBox_PassesStorageIDToService(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := multipartRequest(t, "/import/manabox", "csv content", map[string]string{"storage_id": "7"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastManaBoxStorageID)
	assert.Equal(t, 7, *service.lastManaBoxStorageID)
}

func TestImportManaBox_RejectsInvalidStorageID(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := multipartRequest(t, "/import/manabox", "csv content", map[string]string{"storage_id": "abc"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportManaBox_UnknownStorageReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeImportService{err: ErrTargetStorageNotFound})

	req := multipartRequest(t, "/import/manabox", "csv content", map[string]string{"storage_id": "999"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportMoxfieldCollection_RequiresStorageID(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := multipartRequest(t, "/import/moxfield/collection", "csv content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportMoxfieldCollection_PassesStorageIDToService(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := multipartRequest(t, "/import/moxfield/collection", "csv content", map[string]string{"storage_id": "7"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 7, service.lastMoxfieldStorageID)
}

func TestImportMoxfieldCollection_UnknownStorageReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeImportService{err: ErrTargetStorageNotFound})

	req := multipartRequest(t, "/import/moxfield/collection", "csv content", map[string]string{"storage_id": "999"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportMoxfieldCollection_RequiresFile(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := multipartRequest(t, "/import/moxfield/collection", "", map[string]string{"storage_id": "7"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportMoxfieldCollection_ScryfallFailureReturnsBadGateway(t *testing.T) {
	router := setupRouter(&fakeImportService{err: ErrScryfallUnavailable})

	req := multipartRequest(t, "/import/moxfield/collection", "csv content", map[string]string{"storage_id": "7"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestImportCardList_PassesTheListAndStorage(t *testing.T) {
	service := &fakeImportService{summary: Summary{CardsCreated: 4}}
	router := setupRouter(service)

	req := multipartRequest(t, "/import/list", "4 Lightning Bolt", map[string]string{"storage_id": "3"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "4 Lightning Bolt", service.lastFileContent)
	require.NotNil(t, service.lastListStorageID)
	assert.Equal(t, 3, *service.lastListStorageID)
	assert.Contains(t, w.Body.String(), `"cards_created": 4`)
}

func TestImportCardList_StorageIsOptional(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := multipartRequest(t, "/import/list", "4 Lightning Bolt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, service.lastListStorageID)
}

func TestImportCardList_RejectsBadRequests(t *testing.T) {
	cases := map[string]struct {
		service *fakeImportService
		content string
		fields  map[string]string
	}{
		"invalid storage_id": {&fakeImportService{}, "1 Sol Ring", map[string]string{"storage_id": "x"}},
		"missing file":       {&fakeImportService{}, "", nil},
		"unreadable list":    {&fakeImportService{err: ErrInvalidFile}, "nope", nil},
		"unknown storage":    {&fakeImportService{err: ErrTargetStorageNotFound}, "1 Sol Ring", map[string]string{"storage_id": "9"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := setupRouter(tc.service)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, multipartRequest(t, "/import/list", tc.content, tc.fields))

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestImportMoxfieldDeck_OldRouteIsGone(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := multipartRequest(t, "/import/moxfield/deck", "1 Sol Ring", map[string]string{
		"name": "Pile", "format": "modern",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRespondImport_UnknownErrorReturnsInternalServerError(t *testing.T) {
	router := setupRouter(&fakeImportService{err: assertAnError{}})

	req := multipartRequest(t, "/import/manabox", "content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "something went wrong" }

// --- export routes ---------------------------------------------------------

func TestExportManaBox_ReturnsCSVWithAttachmentHeaders(t *testing.T) {
	service := &fakeImportService{exportContent: "Binder Name,Binder Type\nMain,binder\n"}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/export/manabox", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `attachment; filename="ManaBox_Collection_export.csv"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, "text/csv; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "Binder Name,Binder Type\nMain,binder\n", w.Body.String())
	assert.Equal(t, testUserID, service.lastExportUser)
}

func TestExportManaBox_ServiceErrorReturnsInternalServerError(t *testing.T) {
	router := setupRouter(&fakeImportService{exportErr: assertAnError{}})

	req := httptest.NewRequest(http.MethodGet, "/export/manabox", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestExportMoxfieldCollection_ReturnsCSVWithAttachmentHeaders(t *testing.T) {
	service := &fakeImportService{exportContent: "Count,Name,Edition,Foil,Collector Number\n2,Sol Ring,sld,,1011\n"}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/export/moxfield/collection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `attachment; filename="Moxfield_Collection_export.csv"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, "text/csv; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "Count,Name,Edition,Foil,Collector Number\n2,Sol Ring,sld,,1011\n", w.Body.String())
}

func TestExportMoxfieldCollection_ServiceErrorReturnsInternalServerError(t *testing.T) {
	router := setupRouter(&fakeImportService{exportErr: assertAnError{}})

	req := httptest.NewRequest(http.MethodGet, "/export/moxfield/collection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestExportCollection_PassesTheStorageFilter(t *testing.T) {
	for _, path := range []string{"/export/manabox", "/export/moxfield/collection"} {
		service := &fakeImportService{}
		router := setupRouter(service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+"?storage_id=4", nil))
		require.Equal(t, http.StatusOK, w.Code, path)
		require.NotNil(t, service.lastExportStorageID, path)
		assert.Equal(t, 4, *service.lastExportStorageID, path)

		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code, path)
		assert.Nil(t, service.lastExportStorageID, path)
	}
}

func TestExportCollection_RejectsInvalidStorageID(t *testing.T) {
	for _, path := range []string{"/export/manabox", "/export/moxfield/collection"} {
		router := setupRouter(&fakeImportService{})

		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+"?storage_id=0", nil))

		assert.Equal(t, http.StatusBadRequest, w.Code, path)
	}
}

func TestExportCollection_UnknownStorageReturnsNotFound(t *testing.T) {
	for _, path := range []string{"/export/manabox", "/export/moxfield/collection"} {
		router := setupRouter(&fakeImportService{exportErr: ErrTargetStorageNotFound})

		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+"?storage_id=999", nil))

		assert.Equal(t, http.StatusNotFound, w.Code, path)
	}
}

func TestExportDeck_InvalidIDReturnsBadRequest(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := httptest.NewRequest(http.MethodGet, "/deck/not-a-uuid/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExportDeck_ServiceErrorReturnsInternalServerError(t *testing.T) {
	router := setupRouter(&fakeImportService{exportErr: assertAnError{}})

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestExportDeck_OldMoxfieldRouteIsGone(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := httptest.NewRequest(http.MethodGet, "/export/moxfield/deck/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_RefreshCardDetails_ReturnsSummary(t *testing.T) {
	service := &fakeImportService{refreshSummary: DetailsRefreshSummary{Updated: 3, NotFound: 1}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/cards/refresh-details", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var summary DetailsRefreshSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	assert.Equal(t, DetailsRefreshSummary{Updated: 3, NotFound: 1}, summary)
	assert.Equal(t, testUserID, service.lastUserID)
	assert.Equal(t, 0, service.lastAfterID)
}

func TestHandler_RefreshCardDetails_PassesTheCursor(t *testing.T) {
	next := 42
	service := &fakeImportService{refreshSummary: DetailsRefreshSummary{Updated: 750, Remaining: 10, NextAfterID: &next}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/cards/refresh-details?after_id=17", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 17, service.lastAfterID)
	assert.JSONEq(t, `{"updated":750,"not_found":0,"remaining":10,"next_after_id":42}`, w.Body.String())
}

func TestHandler_RefreshCardDetails_RejectsInvalidCursor(t *testing.T) {
	for _, raw := range []string{"abc", "-1"} {
		router := setupRouter(&fakeImportService{})

		req := httptest.NewRequest(http.MethodPost, "/cards/refresh-details?after_id="+raw, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, raw)
	}
}

func TestHandler_RefreshCardDetails_ReturnsBadGatewayWhenScryfallIsDown(t *testing.T) {
	service := &fakeImportService{err: ErrScryfallUnavailable}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/cards/refresh-details", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestHandler_CommitPendingCards(t *testing.T) {
	service := &fakeImportService{commitSummary: PendingCommitSummary{CardsCreated: 3}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/deck/00000000-0000-0000-0000-000000000009/pending/commit", bytes.NewBufferString(`{"storage_id": 4}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"cards_created": 3}`, w.Body.String())
	assert.Equal(t, "00000000-0000-0000-0000-000000000009", service.lastDeckID)
	require.NotNil(t, service.lastStorageID)
	assert.Equal(t, 4, *service.lastStorageID)
}

func TestHandler_CommitPendingCards_WorksWithoutABody(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/deck/00000000-0000-0000-0000-000000000009/pending/commit", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, service.lastStorageID)
}

func TestHandler_CommitPendingCards_MapsErrors(t *testing.T) {
	cases := map[error]int{ErrDeckNotFound: http.StatusNotFound, ErrTargetStorageNotFound: http.StatusBadRequest}
	for commitErr, expected := range cases {
		router := setupRouter(&fakeImportService{err: commitErr})
		req := httptest.NewRequest(http.MethodPost, "/deck/00000000-0000-0000-0000-000000000009/pending/commit", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, expected, w.Code)
	}
}

func TestHandler_CommitPendingCards_RejectsABadStorageID(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/deck/00000000-0000-0000-0000-000000000009/pending/commit", bytes.NewBufferString(`{"storage_id": 0}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, service.commitCalled)
}

func TestHandler_CommitPendingCards_CanTargetOneCard(t *testing.T) {
	service := &fakeImportService{commitSummary: PendingCommitSummary{CardsCreated: 1}}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/deck/00000000-0000-0000-0000-000000000009/pending/commit", bytes.NewBufferString(`{"pending_id": 5}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastPendingID)
	assert.Equal(t, 5, *service.lastPendingID)
	assert.Nil(t, service.lastStorageID)
}

func TestHandler_CommitPendingCards_ReturnsNotFoundForAnUnknownPendingCard(t *testing.T) {
	router := setupRouter(&fakeImportService{err: deck.ErrPendingCardNotFound})

	req := httptest.NewRequest(http.MethodPost, "/deck/00000000-0000-0000-0000-000000000009/pending/commit", bytes.NewBufferString(`{"pending_id": 5}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestImportIntoDeck_PassesTheDeckAndTheFile(t *testing.T) {
	service := &fakeImportService{summary: Summary{CardsLinked: 2}}
	router := setupRouter(service)

	req := multipartRequest(t, "/deck/00000000-0000-0000-0000-000000000077/import", "2 Sol Ring", map[string]string{
		"commander_from_first_line": "true",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "00000000-0000-0000-0000-000000000077", service.lastDeckID)
	assert.True(t, service.lastIntoDeckCommander)
	assert.Equal(t, "2 Sol Ring", service.lastFileContent)
	assert.Contains(t, w.Body.String(), `"cards_linked": 2`)
}

func TestImportIntoDeck_DefaultsCommanderFromFirstLineToFalse(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := multipartRequest(t, "/deck/00000000-0000-0000-0000-000000000077/import", "2 Sol Ring", map[string]string{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, service.lastIntoDeckCommander)
}

func TestImportIntoDeck_RejectsAnInvalidDeckID(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := multipartRequest(t, "/deck/42/import", "2 Sol Ring", map[string]string{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportIntoDeck_MapsErrors(t *testing.T) {
	for err, status := range map[error]int{
		ErrDeckNotFound:        http.StatusNotFound,
		ErrInvalidFile:         http.StatusBadRequest,
		ErrScryfallUnavailable: http.StatusBadGateway,
	} {
		router := setupRouter(&fakeImportService{err: err})

		req := multipartRequest(t, "/deck/00000000-0000-0000-0000-000000000077/import", "2 Sol Ring", map[string]string{})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, status, w.Code, err.Error())
	}
}

func TestHandler_CommitPendingCards_PassesTheQuantity(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/deck/00000000-0000-0000-0000-000000000009/pending/commit", bytes.NewBufferString(`{"pending_id": 3, "quantity": 5}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, service.lastQuantity)
	assert.Equal(t, 5, *service.lastQuantity)
}

func TestHandler_CommitPendingCards_RejectsAQuantityWithoutAPendingCard(t *testing.T) {
	for _, body := range []string{`{"quantity": 5}`, `{"pending_id": 3, "quantity": 0}`} {
		service := &fakeImportService{}
		router := setupRouter(service)

		req := httptest.NewRequest(http.MethodPost, "/deck/00000000-0000-0000-0000-000000000009/pending/commit", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, body)
		assert.False(t, service.commitCalled)
	}
}

func TestExportDeck_ServesTheChosenFormat(t *testing.T) {
	service := &fakeImportService{exportContent: "1 Sol Ring\n"}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000042/export?format=plain", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1 Sol Ring\n", w.Body.String())
	assert.Equal(t, "plain", service.lastExportFormat)
	assert.Equal(t, "00000000-0000-0000-0000-000000000042", service.lastExportDeck)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "Deck_list.txt")
	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
}

func TestExportDeck_DefaultsToMoxfield(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000042/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "moxfield", service.lastExportFormat)
}

func TestExportDeck_RejectsAnUnknownFormatOrDeck(t *testing.T) {
	router := setupRouter(&fakeImportService{})
	req := httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000042/export?format=mtgo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	router = setupRouter(&fakeImportService{exportErr: ErrDeckNotFound})
	req = httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000042/export?format=arena", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestImportTamiyo_PassesTheFileAndStorage(t *testing.T) {
	service := &fakeImportService{summary: Summary{CardsCreated: 2}}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, multipartRequest(t, "/import/tamiyo", `{"tamiyo": 1}`, map[string]string{"storage_id": "3"}))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"tamiyo": 1}`, service.lastFileContent)
	require.NotNil(t, service.lastTamiyoStorageID)
	assert.Equal(t, 3, *service.lastTamiyoStorageID)
	assert.Contains(t, w.Body.String(), `"cards_created": 2`)
}

func TestImportTamiyo_RejectsBadRequests(t *testing.T) {
	cases := map[string]struct {
		service *fakeImportService
		content string
		fields  map[string]string
	}{
		"invalid storage_id": {&fakeImportService{}, "{}", map[string]string{"storage_id": "x"}},
		"missing file":       {&fakeImportService{}, "", nil},
		"invalid file":       {&fakeImportService{err: ErrInvalidFile}, "nope", nil},
		"deck file":          {&fakeImportService{err: ErrTamiyoDeckFile}, "{}", nil},
		"unknown storage":    {&fakeImportService{err: ErrTargetStorageNotFound}, "{}", map[string]string{"storage_id": "9"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			setupRouter(tc.service).ServeHTTP(w, multipartRequest(t, "/import/tamiyo", tc.content, tc.fields))
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestImportIntoDeck_RefusesATamiyoCollectionFileWithBadRequest(t *testing.T) {
	service := &fakeImportService{err: ErrTamiyoCollectionFile}

	w := httptest.NewRecorder()
	setupRouter(service).ServeHTTP(w, multipartRequest(t, "/deck/00000000-0000-0000-0000-000000000001/import", "{}", nil))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "collection or a storage")
}

func TestExportTamiyoCollection_ReturnsJSONWithAttachmentHeaders(t *testing.T) {
	service := &fakeImportService{exportContent: `{"tamiyo": 1}`}

	w := httptest.NewRecorder()
	setupRouter(service).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/export/tamiyo?storage_id=4", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="Tamiyo_Collection.json"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, `{"tamiyo": 1}`, w.Body.String())
	require.NotNil(t, service.lastExportStorageID)
	assert.Equal(t, 4, *service.lastExportStorageID)
}

func TestExportTamiyoCollection_UnknownStorageReturnsNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	setupRouter(&fakeImportService{exportErr: ErrTargetStorageNotFound}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/export/tamiyo?storage_id=4", nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestExportDeck_TamiyoFormatWithOrWithoutTags(t *testing.T) {
	for query, wantTags := range map[string]bool{"": false, "&tags=true": true, "&tags=false": false} {
		service := &fakeImportService{exportContent: `{"tamiyo": 1}`}

		w := httptest.NewRecorder()
		setupRouter(service).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/export?format=tamiyo"+query, nil))

		require.Equal(t, http.StatusOK, w.Code, query)
		assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, `attachment; filename="Deck_tamiyo.json"`, w.Header().Get("Content-Disposition"))
		assert.Equal(t, DeckExportTamiyo, service.lastExportFormat)
		assert.Equal(t, wantTags, service.lastExportTags, query)
	}
}

func TestExportDeck_RejectsAnInvalidTagsFlag(t *testing.T) {
	w := httptest.NewRecorder()
	setupRouter(&fakeImportService{}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/export?format=tamiyo&tags=maybe", nil))

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_ExportSharedDeck_NeedsNoAccount(t *testing.T) {
	service := &fakeImportService{exportContent: "1 Sol Ring\n"}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service).RegisterPublicRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shared/decks/00000000-0000-0000-0000-000000000001/export?format=tamiyo&tags=true", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1 Sol Ring\n", w.Body.String())
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "Deck_tamiyo.json")
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", service.lastExportDeck)
	assert.True(t, service.lastSharedTags)

	cases := map[string]int{
		"/shared/decks/nope/export": http.StatusNotFound,
		"/shared/decks/00000000-0000-0000-0000-000000000001/export?format=x": http.StatusBadRequest,
	}
	for path, want := range cases {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, want, w.Code, path)
	}

	missing := gin.New()
	NewHandler(&fakeImportService{exportErr: ErrDeckNotFound}).RegisterPublicRoutes(missing)
	w = httptest.NewRecorder()
	missing.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shared/decks/00000000-0000-0000-0000-000000000001/export", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}
