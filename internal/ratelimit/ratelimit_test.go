package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/clientip"
)

func TestLimiter_Allow_AllowsUpToMaxWithinWindow(t *testing.T) {
	l := NewLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		allowed, _ := l.Allow("1.2.3.4")
		require.True(t, allowed, "request %d should be allowed", i+1)
	}
}

func TestLimiter_Allow_RejectsOnceMaxIsExceeded(t *testing.T) {
	l := NewLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		_, _ = l.Allow("1.2.3.4")
	}

	allowed, retryAfter := l.Allow("1.2.3.4")

	assert.False(t, allowed)
	assert.Positive(t, retryAfter)
}

func TestLimiter_Allow_TracksKeysIndependently(t *testing.T) {
	l := NewLimiter(1, time.Minute)

	allowedA, _ := l.Allow("1.2.3.4")
	allowedB, _ := l.Allow("5.6.7.8")

	assert.True(t, allowedA)
	assert.True(t, allowedB)
}

func TestLimiter_Allow_ResetsAfterWindowElapses(t *testing.T) {
	l := NewLimiter(1, 20*time.Millisecond)

	first, _ := l.Allow("1.2.3.4")
	require.True(t, first)

	second, _ := l.Allow("1.2.3.4")
	require.False(t, second)

	time.Sleep(30 * time.Millisecond)

	third, _ := l.Allow("1.2.3.4")
	assert.True(t, third)
}

func TestMiddleware_AllowsRequestsWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware(NewLimiter(2, time.Minute)))
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "request %d", i+1)
	}
}

func TestMiddleware_Returns429WithRetryAfterOnceLimitExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware(NewLimiter(1, time.Minute)))
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	okReq := httptest.NewRequest(http.MethodGet, "/ping", nil)
	okW := httptest.NewRecorder()
	router.ServeHTTP(okW, okReq)
	require.Equal(t, http.StatusOK, okW.Code)

	blockedReq := httptest.NewRequest(http.MethodGet, "/ping", nil)
	blockedW := httptest.NewRecorder()
	router.ServeHTTP(blockedW, blockedReq)

	assert.Equal(t, http.StatusTooManyRequests, blockedW.Code)
	assert.NotEmpty(t, blockedW.Header().Get("Retry-After"))
}

// hit sends one request from the socket address `from` with the given headers and returns the status.
func hit(router *gin.Engine, from string, headers map[string]string) int {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = from + ":4000"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func proxiedRouter(t *testing.T, trusted ...string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, clientip.Configure(router, trusted))
	router.Use(Middleware(NewLimiter(2, time.Minute)))
	router.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func TestMiddleware_UntrustedSocketCountedTogetherDespiteForgedForwardedFor(t *testing.T) {
	router := proxiedRouter(t)

	assert.Equal(t, http.StatusOK, hit(router, "203.0.113.9", map[string]string{"X-Forwarded-For": "1.1.1.1"}))
	assert.Equal(t, http.StatusOK, hit(router, "203.0.113.9", map[string]string{"X-Forwarded-For": "2.2.2.2"}))
	assert.Equal(t, http.StatusTooManyRequests, hit(router, "203.0.113.9", map[string]string{"X-Forwarded-For": "3.3.3.3"}))
}

func TestMiddleware_TrustedProxyCountsUnderAppendedRealIPNotForgedOne(t *testing.T) {
	router := proxiedRouter(t, "10.0.0.0/24")

	assert.Equal(t, http.StatusOK, hit(router, "10.0.0.5", map[string]string{"X-Forwarded-For": "1.1.1.1, 198.51.100.7"}))
	assert.Equal(t, http.StatusOK, hit(router, "10.0.0.5", map[string]string{"X-Forwarded-For": "2.2.2.2, 198.51.100.7"}))
	assert.Equal(t, http.StatusTooManyRequests, hit(router, "10.0.0.5", map[string]string{"X-Forwarded-For": "3.3.3.3, 198.51.100.7"}))
}

func TestMiddleware_ClientsBehindTheSameTrustedProxyHaveSeparateCounters(t *testing.T) {
	router := proxiedRouter(t, "10.0.0.5")

	for i := 0; i < 2; i++ {
		require.Equal(t, http.StatusOK, hit(router, "10.0.0.5", map[string]string{"X-Forwarded-For": "198.51.100.7"}))
	}
	require.Equal(t, http.StatusTooManyRequests, hit(router, "10.0.0.5", map[string]string{"X-Forwarded-For": "198.51.100.7"}))

	assert.Equal(t, http.StatusOK, hit(router, "10.0.0.5", map[string]string{"X-Forwarded-For": "198.51.100.8"}))
}

func TestMiddleware_ForgedXRealIPFromUntrustedAddressChangesNothing(t *testing.T) {
	router := proxiedRouter(t)

	assert.Equal(t, http.StatusOK, hit(router, "203.0.113.9", map[string]string{"X-Real-IP": "1.1.1.1"}))
	assert.Equal(t, http.StatusOK, hit(router, "203.0.113.9", map[string]string{"X-Real-IP": "2.2.2.2"}))
	assert.Equal(t, http.StatusTooManyRequests, hit(router, "203.0.113.9", map[string]string{"X-Real-IP": "3.3.3.3"}))
}

func TestMiddleware_XRealIPIgnoredEvenFromTrustedProxy(t *testing.T) {
	router := proxiedRouter(t, "10.0.0.5")

	assert.Equal(t, http.StatusOK, hit(router, "10.0.0.5", map[string]string{"X-Real-IP": "1.1.1.1"}))
	assert.Equal(t, http.StatusOK, hit(router, "10.0.0.5", map[string]string{"X-Real-IP": "2.2.2.2"}))
	assert.Equal(t, http.StatusTooManyRequests, hit(router, "10.0.0.5", map[string]string{"X-Real-IP": "3.3.3.3"}))
}
