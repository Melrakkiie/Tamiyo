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

	"Melrakkiie/Tamiyo/internal/authcookie"
)

var testCookie = authcookie.New("", true, 24*time.Hour)

type fakeService struct {
	rotateUserID       string
	rotateNewPlaintext string
	rotateErr          error
	rotateCalledWith   string

	revokeErr        error
	revokeCalledWith string
}

func (f *fakeService) Rotate(ctx context.Context, plaintext string) (string, string, error) {
	f.rotateCalledWith = plaintext
	if f.rotateErr != nil {
		return "", "", f.rotateErr
	}
	return f.rotateUserID, f.rotateNewPlaintext, nil
}

func (f *fakeService) Revoke(ctx context.Context, plaintext string) error {
	f.revokeCalledWith = plaintext
	return f.revokeErr
}

func setupRouter(service tokenService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service, "test-secret", time.Hour, testCookie).RegisterRoutes(router)
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

func cookieRequest(path, refreshToken string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.AddCookie(&http.Cookie{Name: authcookie.DefaultName, Value: refreshToken})
	return req
}

func findCookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestHandler_Refresh_ReadsTokenFromCookieWhenBodyIsEmpty(t *testing.T) {
	service := &fakeService{rotateUserID: "11111111-1111-1111-1111-111111111111", rotateNewPlaintext: "new-refresh-token"}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, cookieRequest("/auth/refresh", "cookie-refresh-token"))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "cookie-refresh-token", service.rotateCalledWith)
}

func TestHandler_Refresh_SetsRotatedTokenInCookie(t *testing.T) {
	service := &fakeService{rotateUserID: "11111111-1111-1111-1111-111111111111", rotateNewPlaintext: "new-refresh-token"}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, cookieRequest("/auth/refresh", "cookie-refresh-token"))

	require.Equal(t, http.StatusOK, w.Code)
	cookie := findCookie(w, authcookie.DefaultName)
	require.NotNil(t, cookie)
	assert.Equal(t, "new-refresh-token", cookie.Value)
	assert.True(t, cookie.HttpOnly)
}

func TestHandler_Refresh_PrefersBodyTokenOverCookie(t *testing.T) {
	service := &fakeService{rotateUserID: "11111111-1111-1111-1111-111111111111", rotateNewPlaintext: "new-refresh-token"}
	router := setupRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(`{"refresh_token": "body-refresh-token"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authcookie.DefaultName, Value: "cookie-refresh-token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "body-refresh-token", service.rotateCalledWith)
}

func TestHandler_Refresh_ClearsCookieOnInvalidToken(t *testing.T) {
	service := &fakeService{rotateErr: ErrInvalid}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, cookieRequest("/auth/refresh", "revoked-refresh-token"))

	require.Equal(t, http.StatusUnauthorized, w.Code)
	cookie := findCookie(w, authcookie.DefaultName)
	require.NotNil(t, cookie)
	assert.Less(t, cookie.MaxAge, 0)
}

func TestHandler_Refresh_ReturnsBadRequestWithoutBodyOrCookie(t *testing.T) {
	router := setupRouter(&fakeService{})

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Refresh_ReturnsBadRequestOnMalformedBody(t *testing.T) {
	router := setupRouter(&fakeService{})

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authcookie.DefaultName, Value: "cookie-refresh-token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Logout_RevokesCookieTokenAndClearsCookie(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, cookieRequest("/auth/logout", "cookie-refresh-token"))

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "cookie-refresh-token", service.revokeCalledWith)
	cookie := findCookie(w, authcookie.DefaultName)
	require.NotNil(t, cookie)
	assert.Less(t, cookie.MaxAge, 0)
}
