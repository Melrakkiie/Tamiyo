package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeService struct {
	registerUser User
	registerErr  error

	authUser User
	authErr  error
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

func setupRouter(service userService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service, "test-secret").RegisterRoutes(router)
	return router
}

func TestHandler_Register_ReturnsTokenOnSuccess(t *testing.T) {
	service := &fakeService{registerUser: User{ID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com"}}
	router := setupRouter(service)

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
}

func TestHandler_Register_ReturnsBadRequestOnInvalidEmail(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"email": "not-an-email", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Register_ReturnsBadRequestOnShortPassword(t *testing.T) {
	service := &fakeService{}
	router := setupRouter(service)

	body := `{"email": "alice@example.com", "password": "short"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Register_ReturnsConflictWhenEmailAlreadyTaken(t *testing.T) {
	service := &fakeService{registerErr: ErrEmailAlreadyTaken}
	router := setupRouter(service)

	body := `{"email": "alice@example.com", "password": "supersecret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestHandler_Login_ReturnsTokenOnSuccess(t *testing.T) {
	service := &fakeService{authUser: User{ID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com"}}
	router := setupRouter(service)

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
}

func TestHandler_Login_ReturnsUnauthorizedOnInvalidCredentials(t *testing.T) {
	service := &fakeService{authErr: ErrInvalidCredentials}
	router := setupRouter(service)

	body := `{"email": "alice@example.com", "password": "wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_RegisterRoutes_AppliesGivenMiddlewareToBothAuthRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	blockAll := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests, please try again later"})
	}

	NewHandler(&fakeService{}, "test-secret").RegisterRoutes(router, blockAll)

	for _, path := range []string{"/auth/register", "/auth/login"} {
		body := `{"email": "alice@example.com", "password": "supersecret"}`
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTooManyRequests, w.Code, "path %s should go through the middleware", path)
	}
}
