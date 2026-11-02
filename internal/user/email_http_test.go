package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/auth"
)

// memRepository keeps accounts in memory and, like the database index, refuses a case variant of a stored address.
type memRepository struct {
	Repository
	users []User
}

func (m *memRepository) Create(_ context.Context, u User) (User, error) {
	for _, existing := range m.users {
		if strings.EqualFold(existing.Email, u.Email) {
			return User{}, ErrEmailAlreadyTaken
		}
	}
	u.ID = testUserID
	m.users = append(m.users, u)
	return u, nil
}

func (m *memRepository) FindByEmail(_ context.Context, email string) (User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

func (m *memRepository) FindByID(_ context.Context, id string) (User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

func postJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func realRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(NewService(&memRepository{}), testJWTSecret, testAccessTokenTTL, &fakeTokenService{issuedRefreshToken: "r"}, testCookie)
	handler.RegisterRoutes(router)
	protected := router.Group("/")
	protected.Use(auth.RequireAuth(testJWTSecret))
	handler.RegisterProtectedRoutes(protected)
	return router
}

func TestHandler_Email_LoginIgnoresCaseAndSurroundingSpaces(t *testing.T) {
	router := realRouter()
	require.Equal(t, http.StatusCreated, postJSON(router, "/auth/register", `{"email": "Bob@Mail.fr", "password": "supersecret"}`).Code)

	assert.Equal(t, http.StatusOK, postJSON(router, "/auth/login", `{"email": "bob@mail.fr", "password": "supersecret"}`).Code)
	assert.Equal(t, http.StatusOK, postJSON(router, "/auth/login", `{"email": "  BOB@mail.FR ", "password": "supersecret"}`).Code)
}

func TestHandler_Email_RegisteringACaseVariantIs409(t *testing.T) {
	router := realRouter()
	require.Equal(t, http.StatusCreated, postJSON(router, "/auth/register", `{"email": "Bob@mail.fr", "password": "supersecret"}`).Code)

	assert.Equal(t, http.StatusConflict, postJSON(router, "/auth/register", `{"email": "bob@mail.fr", "password": "supersecret"}`).Code)
}

func TestHandler_Email_MeReturnsTheLowercasedAddress(t *testing.T) {
	router := realRouter()
	w := postJSON(router, "/auth/register", `{"email": " Bob@Mail.fr ", "password": "supersecret"}`)
	require.Equal(t, http.StatusCreated, w.Code)
	var tokens map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tokens))

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokens["token"])
	me := httptest.NewRecorder()
	router.ServeHTTP(me, req)

	require.Equal(t, http.StatusOK, me.Code)
	assert.Contains(t, me.Body.String(), `"email":"bob@mail.fr"`)
}
