// Package bodylimit caps the size of every request body.
package bodylimit

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Middleware refuses a request declaring more than limit bytes with 413, and
// cuts off any other body (chunked, or lying about its length) at limit bytes.
func Middleware(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
