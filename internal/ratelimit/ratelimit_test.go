package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
