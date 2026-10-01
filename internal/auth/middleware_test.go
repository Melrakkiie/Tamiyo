package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProtectedRouter(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", RequireAuth(secret), func(c *gin.Context) {
		userID, _ := UserIDFromContext(c)
		c.JSON(http.StatusOK, gin.H{"user_id": userID})
	})
	return router
}

func TestRequireAuth_AllowsRequestWithValidToken(t *testing.T) {
	router := setupProtectedRouter("test-secret")
	token, _ := GenerateToken("test-secret", "11111111-1111-1111-1111-111111111111", time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_RejectsMissingHeader(t *testing.T) {
	router := setupProtectedRouter("test-secret")

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_RejectsMalformedHeader(t *testing.T) {
	router := setupProtectedRouter("test-secret")

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "NotBearer sometoken")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_RejectsInvalidToken(t *testing.T) {
	router := setupProtectedRouter("test-secret")

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
