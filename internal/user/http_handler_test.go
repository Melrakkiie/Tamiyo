package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/authcookie"
)

const testJWTSecret = "test-secret"
const testAccessTokenTTL = time.Hour

var testCookie = authcookie.New("", true, 24*time.Hour)

type fakeService struct {
	registerUser User
	registerErr  error

	authUser User
	authErr  error

	changePasswordErr error

	changePasswordCalledWithUserID string
	changePasswordCalledWithOld    string
	changePasswordCalledWithNew    string
}

func (f *fakeService) Register(ctx context.Context, email, password string) (User, error) {
	if f.registerErr != nil {
		return User{}, f.registerErr
	}
	return f.registerUser, nil
}

func (f *fakeService) Authenticate(ctx context.Context, email, password string) (User, error) {
	if f.authErr != nil {
		return User{}, f.authErr
	}
	return f.authUser, nil
}

func (f *fakeService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	f.changePasswordCalledWithUserID = userID
	f.changePasswordCalledWithOld = currentPassword
	f.changePasswordCalledWithNew = newPassword
	return f.changePasswordErr
}

type fakeTokenService struct {
	issuedRefreshToken    string
	issueErr              error
	issueCalledWithUserID string

	revokeAllErr              error
	revokeAllCalledWithUserID string
}

func (f *fakeTokenService) IssueRefreshToken(ctx context.Context, userID string) (string, error) {
	f.issueCalledWithUserID = userID
	if f.issueErr != nil {
		return "", f.issueErr
	}
	if f.issuedRefreshToken == "" {
		return "fake-refresh-token", nil
	}
	return f.issuedRefreshToken, nil
}

func (f *fakeTokenService) RevokeAllForUser(ctx context.Context, userID string) error {
	f.revokeAllCalledWithUserID = userID
	return f.revokeAllErr
}

func setupRouter(service userService, tokens refreshTokenService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service, testJWTSecret, testAccessTokenTTL, tokens, testCookie).RegisterRoutes(router)
	return router
}

func setupProtectedRouter(service userService, tokens refreshTokenService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/")
	protected.Use(auth.RequireAuth(testJWTSecret))
	NewHandler(service, testJWTSecret, testAccessTokenTTL, tokens, testCookie).RegisterProtectedRoutes(protected)
	return router
}

func authenticatedRequest(method, path, body, userID string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	token, _ := auth.GenerateToken(testJWTSecret, userID, time.Hour)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestHandler_Register_ReturnsTokenPairOnSuccess(t *testing.T) {
	service := &fakeService{registerUser: User{ID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com"}}
	tokens := &fakeTokenService{issuedRefreshToken: "a-refresh-token"}
	router := setupRouter(service, tokens)

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["token"])
	assert.Equal(t, "a-refresh-token", response["refresh_token"])
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", tokens.issueCalledWithUserID)
}

func TestHandler_Register_SetsRefreshCookie(t *testing.T) {
	service := &fakeService{registerUser: User{ID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com"}}
	tokens := &fakeTokenService{issuedRefreshToken: "a-refresh-token"}
	router := setupRouter(service, tokens)

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	cookie := findCookie(w, authcookie.DefaultName)
	require.NotNil(t, cookie)
	assert.Equal(t, "a-refresh-token", cookie.Value)
	assert.True(t, cookie.HttpOnly)
}

func TestHandler_Register_ReturnsBadRequestOnInvalidEmail(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service, &fakeTokenService{})

	body := `{"email": "not-an-email", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Register_ReturnsBadRequestOnShortPassword(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service, &fakeTokenService{})

	body := `{"email": "alice@example.com", "password": "short"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Register_ReturnsConflictWhenEmailAlreadyTaken(t *testing.T) {
	service := &fakeService{registerErr: ErrEmailAlreadyTaken}
	router := setupRouter(service, &fakeTokenService{})

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestHandler_Register_ReturnsInternalServerErrorWhenIssuingRefreshTokenFails(t *testing.T) {
	service := &fakeService{registerUser: User{ID: "11111111-1111-1111-1111-111111111111"}}
	tokens := &fakeTokenService{issueErr: assertAnError}
	router := setupRouter(service, tokens)

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_Login_ReturnsTokenPairOnSuccess(t *testing.T) {
	service := &fakeService{authUser: User{ID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com"}}
	tokens := &fakeTokenService{issuedRefreshToken: "a-refresh-token"}
	router := setupRouter(service, tokens)

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["token"])
	assert.Equal(t, "a-refresh-token", response["refresh_token"])
}

func TestHandler_Login_SetsRefreshCookie(t *testing.T) {
	service := &fakeService{authUser: User{ID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com"}}
	tokens := &fakeTokenService{issuedRefreshToken: "a-refresh-token"}
	router := setupRouter(service, tokens)

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	cookie := findCookie(w, authcookie.DefaultName)
	require.NotNil(t, cookie)
	assert.Equal(t, "a-refresh-token", cookie.Value)
}

func TestHandler_Login_OmitsRefreshTokenFromBodyForCookieOnlyClients(t *testing.T) {
	service := &fakeService{authUser: User{ID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com"}}
	tokens := &fakeTokenService{issuedRefreshToken: "a-refresh-token"}
	router := setupRouter(service, tokens)

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authcookie.TransportHeader, "cookie")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.NotEmpty(t, response["token"])
	_, hasRefreshToken := response["refresh_token"]
	assert.False(t, hasRefreshToken)

	cookie := findCookie(w, authcookie.DefaultName)
	require.NotNil(t, cookie)
	assert.Equal(t, "a-refresh-token", cookie.Value)
}

func TestHandler_Login_DoesNotSetRefreshCookieOnInvalidCredentials(t *testing.T) {
	service := &fakeService{authErr: ErrInvalidCredentials}
	router := setupRouter(service, &fakeTokenService{})

	body := `{"email": "alice@example.com", "password": "wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, findCookie(w, authcookie.DefaultName))
}

func TestHandler_Login_ReturnsUnauthorizedOnInvalidCredentials(t *testing.T) {
	service := &fakeService{authErr: ErrInvalidCredentials}
	router := setupRouter(service, &fakeTokenService{})

	body := `{"email": "alice@example.com", "password": "wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_Login_ReturnsBadRequestOnInvalidBody(t *testing.T) {
	router := setupRouter(&fakeService{}, &fakeTokenService{})

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Login_ReturnsInternalServerErrorOnUnknownAuthError(t *testing.T) {
	service := &fakeService{authErr: errors.New("boom")}
	router := setupRouter(service, &fakeTokenService{})

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_Login_ReturnsInternalServerErrorWhenRefreshTokenIssueFails(t *testing.T) {
	service := &fakeService{authUser: User{ID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com"}}
	tokens := &fakeTokenService{issueErr: errors.New("boom")}
	router := setupRouter(service, tokens)

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_RegisterRoutes_AppliesGivenMiddlewareToBothAuthRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	blockAll := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests, please try again later"})
	}

	NewHandler(&fakeService{}, testJWTSecret, testAccessTokenTTL, &fakeTokenService{}, testCookie).RegisterRoutes(router, blockAll)

	for _, path := range []string{"/auth/register", "/auth/login"} {
		body := `{"email": "alice@example.com", "password": "supersecret"}`
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTooManyRequests, w.Code, "path %s should go through the middleware", path)
	}
}

func TestHandler_ChangePassword_ReturnsNoContentOnSuccess(t *testing.T) {
	service := &fakeService{}
	router := setupProtectedRouter(service, &fakeTokenService{})

	body := `{"current_password": "oldpassword", "new_password": "newpassword"}`
	req := authenticatedRequest(http.MethodPost, "/auth/password", body, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", service.changePasswordCalledWithUserID)
	assert.Equal(t, "oldpassword", service.changePasswordCalledWithOld)
	assert.Equal(t, "newpassword", service.changePasswordCalledWithNew)
}

func TestHandler_ChangePassword_RevokesAllRefreshTokensOnSuccess(t *testing.T) {
	service := &fakeService{}
	tokens := &fakeTokenService{}
	router := setupProtectedRouter(service, tokens)

	body := `{"current_password": "oldpassword", "new_password": "newpassword"}`
	req := authenticatedRequest(http.MethodPost, "/auth/password", body, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", tokens.revokeAllCalledWithUserID)
}

func TestHandler_ChangePassword_ReturnsInternalServerErrorWhenRevokeAllFails(t *testing.T) {
	service := &fakeService{}
	tokens := &fakeTokenService{revokeAllErr: assertAnError}
	router := setupProtectedRouter(service, tokens)

	body := `{"current_password": "oldpassword", "new_password": "newpassword"}`
	req := authenticatedRequest(http.MethodPost, "/auth/password", body, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_ChangePassword_ReturnsUnauthorizedOnIncorrectCurrentPassword(t *testing.T) {
	service := &fakeService{changePasswordErr: ErrIncorrectPassword}
	router := setupProtectedRouter(service, &fakeTokenService{})

	body := `{"current_password": "wrongpassword", "new_password": "newpassword"}`
	req := authenticatedRequest(http.MethodPost, "/auth/password", body, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_ChangePassword_ReturnsNotFoundWhenUserNoLongerExists(t *testing.T) {
	service := &fakeService{changePasswordErr: ErrNotFound}
	router := setupProtectedRouter(service, &fakeTokenService{})

	body := `{"current_password": "oldpassword", "new_password": "newpassword"}`
	req := authenticatedRequest(http.MethodPost, "/auth/password", body, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_ChangePassword_ReturnsBadRequestOnShortNewPassword(t *testing.T) {
	service := &fakeService{}
	router := setupProtectedRouter(service, &fakeTokenService{})

	body := `{"current_password": "oldpassword", "new_password": "short"}`
	req := authenticatedRequest(http.MethodPost, "/auth/password", body, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_ChangePassword_ReturnsBadRequestWhenCurrentPasswordMissing(t *testing.T) {
	service := &fakeService{}
	router := setupProtectedRouter(service, &fakeTokenService{})

	body := `{"new_password": "newpassword"}`
	req := authenticatedRequest(http.MethodPost, "/auth/password", body, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_ChangePassword_ReturnsUnauthorizedWithoutToken(t *testing.T) {
	service := &fakeService{}
	router := setupProtectedRouter(service, &fakeTokenService{})

	body := `{"current_password": "oldpassword", "new_password": "newpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

var assertAnError = errAnError{}

type errAnError struct{}

func (errAnError) Error() string { return "boom" }

func findCookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}
