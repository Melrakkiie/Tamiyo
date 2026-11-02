package emailverification

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type fakeVerifier struct {
	err    error
	tokens []string
}

func (f *fakeVerifier) Verify(ctx context.Context, plaintext string) error {
	f.tokens = append(f.tokens, plaintext)
	return f.err
}

func postVerify(verifier *fakeVerifier, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(verifier).RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/auth/verify-email", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHandler_VerifyEmail_Returns204(t *testing.T) {
	verifier := &fakeVerifier{}

	w := postVerify(verifier, `{"token":"abc"}`)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, []string{"abc"}, verifier.tokens)
}

func TestHandler_VerifyEmail_InvalidTokenIs400(t *testing.T) {
	w := postVerify(&fakeVerifier{err: ErrInvalid}, `{"token":"abc"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_VerifyEmail_MissingTokenIs400(t *testing.T) {
	verifier := &fakeVerifier{}

	w := postVerify(verifier, `{}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, verifier.tokens)
}
