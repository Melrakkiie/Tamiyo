package apierr

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errNotFound = errors.New("not found")
	errConflict = errors.New("conflict")
)

func respond(t *testing.T, err error, mappings ...Mapping) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Respond(ctx, err, mappings...)

	return w
}

func TestRespond_UsesStatusAndMessageOfFirstMatchingMapping(t *testing.T) {
	w := respond(t, errNotFound,
		Mapping{Err: errNotFound, Status: http.StatusNotFound, Message: "thing not found"},
		Mapping{Err: errConflict, Status: http.StatusConflict, Message: "already exists"},
	)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "thing not found", body["error"])
}

func TestRespond_MatchesWrappedErrorsViaErrorsIs(t *testing.T) {
	wrapped := errors.New("loading widget: " + errNotFound.Error())
	wrapped = errorsJoinForTest(errNotFound, wrapped)

	w := respond(t, wrapped, Mapping{Err: errNotFound, Status: http.StatusNotFound, Message: "not found"})

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRespond_FallsBackToErrErrorWhenMappingMessageIsEmpty(t *testing.T) {
	w := respond(t, errConflict, Mapping{Err: errConflict, Status: http.StatusConflict})

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, errConflict.Error(), body["error"])
}

func TestRespond_TriesMappingsInOrderAndUsesFirstMatch(t *testing.T) {
	w := respond(t, errConflict,
		Mapping{Err: errConflict, Status: http.StatusConflict, Message: "first"},
		Mapping{Err: errConflict, Status: http.StatusBadRequest, Message: "second"},
	)

	assert.Equal(t, http.StatusConflict, w.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "first", body["error"])
}

func TestRespond_AnswersGeneric500WithoutLeakingErrorWhenNoMappingMatches(t *testing.T) {
	boom := errors.New(`pq: duplicate key value violates unique constraint "cards_secret_idx"`)
	w := respond(t, boom, Mapping{Err: errNotFound, Status: http.StatusNotFound})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error": "internal server error"}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), "cards_secret_idx")
}

func TestRespond_AttachesUnmatchedErrorToGinContextForAccessLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	boom := errors.New("db is down")
	Respond(ctx, boom)

	require.Len(t, ctx.Errors, 1)
	assert.ErrorIs(t, ctx.Errors[0].Err, boom)
}

func TestRespond_DoesNotAttachErrorToGinContextWhenAMappingMatchesWithA4xxStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Respond(ctx, errNotFound, Mapping{Err: errNotFound, Status: http.StatusNotFound, Message: "not found"})

	assert.Empty(t, ctx.Errors)
}

func TestRespond_AttachesErrorToGinContextWhenAMappingMatchesWithA5xxStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	upstream := errorsJoinForTest(errConflict, errors.New("upstream answered 503"))
	Respond(ctx, upstream, Mapping{Err: errConflict, Status: http.StatusBadGateway, Message: "upstream unavailable"})

	assert.Equal(t, http.StatusBadGateway, w.Code)
	require.Len(t, ctx.Errors, 1)
	assert.ErrorIs(t, ctx.Errors[0].Err, upstream)
}

func errorsJoinForTest(sentinel error, msg error) error {
	return &wrappedErr{sentinel: sentinel, msg: msg}
}

type wrappedErr struct {
	sentinel error
	msg      error
}

func (w *wrappedErr) Error() string { return w.msg.Error() }
func (w *wrappedErr) Unwrap() error { return w.sentinel }
