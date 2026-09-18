package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePinger struct {
	pingErr error
}

func (f *fakePinger) PingContext(ctx context.Context) error {
	return f.pingErr
}

func setupRouter(db pinger) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(db).RegisterRoutes(router)
	return router
}

func TestHandler_Check_ReturnsOKWhenDatabaseIsReachable(t *testing.T) {
	router := setupRouter(&fakePinger{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response["status"])
}

func TestHandler_Check_ReturnsServiceUnavailableWhenDatabaseIsUnreachable(t *testing.T) {
	router := setupRouter(&fakePinger{pingErr: errors.New("connection refused")})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "unavailable", response["status"])
}
