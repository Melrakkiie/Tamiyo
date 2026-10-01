package httplog

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return zap.New(core), logs
}

func setupRouter(logger *zap.Logger, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery(logger))
	router.Use(Middleware(logger))
	router.GET("/test", handler)
	return router
}

func doRequest(router *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/test?foo=bar", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestMiddleware_LogsInfoOnSuccess(t *testing.T) {
	logger, logs := newObservedLogger()
	router := setupRouter(logger, func(c *gin.Context) { c.Status(http.StatusOK) })

	w := doRequest(router)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, zap.InfoLevel, entry.Level)
	fields := entry.ContextMap()
	assert.Equal(t, "GET", fields["method"])
	assert.Equal(t, "/test?foo=bar", fields["path"])
	assert.EqualValues(t, http.StatusOK, fields["status"])
	assert.NotContains(t, fields, "error")
}

func TestMiddleware_LogsWarnOnClientError(t *testing.T) {
	logger, logs := newObservedLogger()
	router := setupRouter(logger, func(c *gin.Context) { c.Status(http.StatusBadRequest) })

	doRequest(router)

	require.Equal(t, 1, logs.Len())
	assert.Equal(t, zap.WarnLevel, logs.All()[0].Level)
}

func TestMiddleware_LogsErrorOnServerErrorAndIncludesAttachedError(t *testing.T) {
	logger, logs := newObservedLogger()
	router := setupRouter(logger, func(c *gin.Context) {
		_ = c.Error(errors.New("db connection lost"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	})

	doRequest(router)

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, zap.ErrorLevel, entry.Level)
	fields := entry.ContextMap()
	assert.Contains(t, fields["error"], "db connection lost")
}

func TestRecovery_RecoversPanicLogsAndRespondsWithGenericError(t *testing.T) {
	logger, logs := newObservedLogger()
	router := setupRouter(logger, func(c *gin.Context) {
		panic("something exploded")
	})

	w := doRequest(router)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error": "internal server error"}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), "something exploded")

	require.GreaterOrEqual(t, logs.Len(), 1)
	panicEntry := logs.All()[0]
	assert.Equal(t, zap.ErrorLevel, panicEntry.Level)
	assert.Equal(t, "panic recovered", panicEntry.Message)
	assert.Equal(t, "something exploded", panicEntry.ContextMap()["panic"])
}
