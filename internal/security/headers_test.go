package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Headers())
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func doRequest(t *testing.T, router *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHeaders_SetsXContentTypeOptions(t *testing.T) {
	w := doRequest(t, setupRouter())

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}

func TestHeaders_SetsXFrameOptions(t *testing.T) {
	w := doRequest(t, setupRouter())

	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
}

func TestHeaders_SetsReferrerPolicy(t *testing.T) {
	w := doRequest(t, setupRouter())

	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
}

func TestHeaders_SetsContentSecurityPolicy(t *testing.T) {
	w := doRequest(t, setupRouter())

	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", w.Header().Get("Content-Security-Policy"))
}

func TestHeaders_DoesNotBlockTheRequest(t *testing.T) {
	w := doRequest(t, setupRouter())

	assert.Equal(t, http.StatusOK, w.Code)
}
