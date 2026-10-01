package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const userIDContextKey = "user_id"

// RequireAuth validates the Bearer token on every request and injects the
// authenticated user's ID into the request context. Requests without a
// valid token never reach the handler.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
			return
		}

		tokenString := strings.TrimPrefix(header, "Bearer ")
		userID, err := ParseToken(secret, tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set(userIDContextKey, userID)
		c.Next()
	}
}

// UserIDFromContext retrieves the authenticated user's ID, set earlier by
// RequireAuth. The second return value is false if called outside a route
// protected by RequireAuth.
func UserIDFromContext(c *gin.Context) (string, bool) {
	raw, exists := c.Get(userIDContextKey)
	if !exists {
		return "", false
	}
	id, ok := raw.(string)
	return id, ok
}
