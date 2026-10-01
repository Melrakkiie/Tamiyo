package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRouter(allowedOrigins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware(allowedOrigins))
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func TestMiddleware_PassesThroughRequestsWithoutOriginHeader(t *testing.T) {
	router := setupRouter([]string{"https://tamiyo.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestMiddleware_SetsAllowOriginForAllowedOrigin(t *testing.T) {
	router := setupRouter([]string{"https://tamiyo.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://tamiyo.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://tamiyo.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", w.Header().Get("Vary"))
}

func TestMiddleware_DoesNotSetAllowOriginForDisallowedOrigin(t *testing.T) {
	router := setupRouter([]string{"https://tamiyo.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "the request itself still succeeds; only the browser-side read is blocked, by the header's absence")
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestMiddleware_NeverSetsAllowCredentials(t *testing.T) {
	router := setupRouter([]string{"https://tamiyo.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://tamiyo.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestMiddleware_RespondsToPreflightForAllowedOriginWithNoContent(t *testing.T) {
	router := setupRouter([]string{"https://tamiyo.example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://tamiyo.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://tamiyo.example.com", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestMiddleware_PreflightIncludesAllowedMethodsHeadersAndMaxAge(t *testing.T) {
	router := setupRouter([]string{"https://tamiyo.example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://tamiyo.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "PATCH")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.NotEmpty(t, w.Header().Get("Access-Control-Max-Age"))
}

func TestMiddleware_PreflightForDisallowedOriginReturnsNoContentWithoutAllowOrigin(t *testing.T) {
	router := setupRouter([]string{"https://tamiyo.example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestMiddleware_TreatsEachConfiguredOriginIndependently(t *testing.T) {
	router := setupRouter([]string{"https://tamiyo.example.com", "http://localhost:5173"})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestMiddleware_WithNoAllowedOriginsConfiguredAllowsNothing(t *testing.T) {
	router := setupRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://tamiyo.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}
