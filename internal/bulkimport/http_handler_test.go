package bulkimport

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
)

type fakeImportService struct {
	summary Summary
	err     error

	lastUserID            string
	lastMoxfieldStorageID int
	lastDeckRequest       MoxfieldDeckImportRequest
	lastFileContent       string
}

func (f *fakeImportService) ImportManaBox(ctx context.Context, userID string, r io.Reader) (Summary, error) {
	f.lastUserID = userID
	f.readFile(r)
	return f.summary, f.err
}

func (f *fakeImportService) ImportMoxfieldCollection(ctx context.Context, userID string, storageID int, r io.Reader) (Summary, error) {
	f.lastUserID = userID
	f.lastMoxfieldStorageID = storageID
	f.readFile(r)
	return f.summary, f.err
}

func (f *fakeImportService) ImportMoxfieldDeck(ctx context.Context, userID string, req MoxfieldDeckImportRequest, r io.Reader) (Summary, error) {
	f.lastUserID = userID
	f.lastDeckRequest = req
	f.readFile(r)
	return f.summary, f.err
}

func (f *fakeImportService) readFile(r io.Reader) {
	b, _ := io.ReadAll(r)
	f.lastFileContent = string(b)
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

func TestImportMoxfieldCollection_ScryfallFailureReturnsBadGateway(t *testing.T) {
	router := setupRouter(&fakeImportService{err: ErrScryfallUnavailable})

	req := multipartRequest(t, "/import/moxfield/collection", "csv content", map[string]string{"storage_id": "7"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestImportMoxfieldDeck_RequiresNameAndFormat(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := multipartRequest(t, "/import/moxfield/deck", "deck content", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportMoxfieldDeck_DefaultsCommanderFromFirstLineToTrue(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := multipartRequest(t, "/import/moxfield/deck", "deck content", map[string]string{
		"name": "My Deck", "format": "commander",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "My Deck", service.lastDeckRequest.Name)
	assert.Equal(t, "commander", service.lastDeckRequest.Format)
	assert.True(t, service.lastDeckRequest.CommanderFromFirstLine)
	assert.Nil(t, service.lastDeckRequest.StorageID)
}

func TestImportMoxfieldDeck_CanDisableCommanderFromFirstLine(t *testing.T) {
	service := &fakeImportService{}
	router := setupRouter(service)

	req := multipartRequest(t, "/import/moxfield/deck", "deck content", map[string]string{
		"name": "Pile", "format": "modern", "commander_from_first_line": "false", "storage_id": "5",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, service.lastDeckRequest.CommanderFromFirstLine)
	require.NotNil(t, service.lastDeckRequest.StorageID)
	assert.Equal(t, 5, *service.lastDeckRequest.StorageID)
}

func TestImportMoxfieldDeck_RejectsInvalidBoolean(t *testing.T) {
	router := setupRouter(&fakeImportService{})

	req := multipartRequest(t, "/import/moxfield/deck", "deck content", map[string]string{
		"name": "Pile", "format": "modern", "commander_from_first_line": "not-a-bool",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
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
