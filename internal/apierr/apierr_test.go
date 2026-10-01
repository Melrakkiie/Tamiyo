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

func TestRespond_FallsBackTo500AndAttachesErrorWhenNoMappingMatches(t *testing.T) {
	boom := errors.New("db is down")
	w := respond(t, boom, Mapping{Err: errNotFound, Status: http.StatusNotFound})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "db is down", body["error"])
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

func TestRespond_DoesNotAttachErrorToGinContextWhenAMappingMatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Respond(ctx, errNotFound, Mapping{Err: errNotFound, Status: http.StatusNotFound, Message: "not found"})

	assert.Empty(t, ctx.Errors)
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
