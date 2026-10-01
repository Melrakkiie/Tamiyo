package cors

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	allowedMethods = "GET, POST, PATCH, DELETE, OPTIONS"
	allowedHeaders = "Authorization, Content-Type"
	maxAgeSeconds  = "600"
)

func Middleware(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		if _, ok := allowed[origin]; !ok {
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin")

		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", allowedMethods)
			c.Header("Access-Control-Allow-Headers", allowedHeaders)
			c.Header("Access-Control-Max-Age", maxAgeSeconds)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
