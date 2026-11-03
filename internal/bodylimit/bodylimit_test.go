package bodylimit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type result struct {
	ran  bool
	body string
	err  error
}

func serve(limit int64, req *http.Request) (*httptest.ResponseRecorder, *result) {
	gin.SetMode(gin.TestMode)
	res := &result{}
	r := gin.New()
	r.Use(Middleware(limit))
	r.POST("/", func(c *gin.Context) {
		res.ran = true
		b, err := io.ReadAll(c.Request.Body)
		res.body, res.err = string(b), err
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w, res
}

func TestMiddleware_RejectsDeclaredBodyOverLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 11)))

	w, res := serve(10, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.JSONEq(t, `{"error": "request body too large"}`, w.Body.String())
	assert.False(t, res.ran)
}

func TestMiddleware_PassesBodyAtLimitIntact(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 10)))

	w, res := serve(10, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, strings.Repeat("a", 10), res.body)
	require.NoError(t, res.err)
}

func TestMiddleware_CutsOffChunkedBodyAtLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 50)))
	req.ContentLength = -1

	_, res := serve(10, req)

	assert.True(t, res.ran)
	require.Error(t, res.err)
	assert.LessOrEqual(t, len(res.body), 10)
}
