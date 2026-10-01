package ratelimit

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Limiter is a simple in-memory fixed-window rate limiter, keyed by an
// arbitrary string (typically the client IP). It allows up to max requests
// per key within window, then rejects further requests until the window
// resets.
//
// It is intentionally minimal: no external dependency, safe for concurrent
// use, good enough to blunt naive brute-force attempts against a
// single-instance deployment. It does NOT share state across replicas — if
// Tamiyo is ever run with more than one instance behind a load balancer,
// this needs to move to a shared store (e.g. Redis) or every instance will
// enforce its own independent limit.
type Limiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	visitors map[string]*visitor
}

type visitor struct {
	count   int
	resetAt time.Time
}

// NewLimiter creates a Limiter allowing up to max requests per key every
// window. It starts a background goroutine that periodically evicts expired
// entries so memory usage stays bounded over the life of the process.
func NewLimiter(max int, window time.Duration) *Limiter {
	l := &Limiter{
		max:      max,
		window:   window,
		visitors: make(map[string]*visitor),
	}
	go l.cleanupLoop()
	return l
}

// Allow reports whether a request identified by key is allowed under the
// current window. When it is not, the second return value is how long the
// caller should wait before retrying.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	v, ok := l.visitors[key]
	if !ok || now.After(v.resetAt) {
		l.visitors[key] = &visitor{count: 1, resetAt: now.Add(l.window)}
		return true, 0
	}

	if v.count >= l.max {
		return false, time.Until(v.resetAt)
	}

	v.count++
	return true, 0
}

// cleanupLoop periodically evicts expired visitors so the map doesn't grow
// unbounded over the life of the process.
func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for key, v := range l.visitors {
			if now.After(v.resetAt) {
				delete(l.visitors, key)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware returns a gin.HandlerFunc that rate-limits requests by client
// IP, responding 429 Too Many Requests with a Retry-After header (in
// seconds) once the limit is reached.
func Middleware(l *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, retryAfter := l.Allow(c.ClientIP())
		if !allowed {
			secs := int(retryAfter.Seconds())
			if secs < 1 {
				secs = 1
			}
			c.Header("Retry-After", strconv.Itoa(secs))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests, please try again later",
			})
			return
		}
		c.Next()
	}
}
