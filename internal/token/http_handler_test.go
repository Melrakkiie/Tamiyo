package token

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeService struct {
	rotateUserID       string
	rotateNewPlaintext string
	rotateErr          error

	revokeErr error
}

func (f *fakeService) Rotate(ctx context.Context, plaintext string) (string, string, error) {
	if f.rotateErr != nil {
		return "", "", f.rotateErr
	}
	return f.rotateUserID, f.rotateNewPlaintext, nil
}

func (f *fakeService) Revoke(ctx context.Context, plaintext string) error {
	return f.revokeErr
}

func setupRouter(service tokenService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service, "test-secret", time.Hour).RegisterRoutes(router)
	return router
}

func TestHandler_Refresh_ReturnsNewTokenPairOnSuccess(t *testing.T) {
	service := &fakeService{rotateUserID: "11111111-1111-1111-1111-111111111111", rotateNewPlaintext: "new-refresh-token"}
	router := setupRouter(service)

	body := `{"refresh_token": "old-refresh-token"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["token"])
	assert.Equal(t, "new-refresh-token", response["refresh_token"])
}

func TestHandler_Refresh_ReturnsUnauthorizedOnInvalidToken(t *testing.T) {
	service := &fakeService{rotateErr: ErrInvalid}
	router := setupRouter(service)

	body := `{"refresh_token": "bogus"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_Refresh_ReturnsBadRequestWhenTokenMissing(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Logout_ReturnsNoContentOnSuccess(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"refresh_token": "some-refresh-token"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandler_Logout_ReturnsBadRequestWhenTokenMissing(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
