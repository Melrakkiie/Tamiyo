// Package bodylimit caps the size of every request body.
package bodylimit

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Middleware refuses a request declaring more than limit bytes with 413, and
// cuts off any other body (chunked, or lying about its length) at limit bytes.
// Multipart uploads are only capped: their routes answer an oversized body
// with their own 413 message.
func Middleware(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		multipart := strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data")
		if c.Request.ContentLength > limit && !multipart {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
