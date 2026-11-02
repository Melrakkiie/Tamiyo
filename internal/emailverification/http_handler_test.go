package emailverification

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/user"
)

const (
	testJWTSecret = "test-secret"
	testUserID    = "11111111-1111-1111-1111-111111111111"
)

type fakeAccounts struct {
	user user.User
	err  error
}

func (f *fakeAccounts) GetUser(ctx context.Context, userID string) (user.User, error) {
	return f.user, f.err
}

type fakeVerifier struct {
	err    error
	tokens []string

	issueErr   error
	issuedFor  string
	issuedMail string
	issueCalls int
}

func (f *fakeVerifier) IssueAndSend(ctx context.Context, userID, email string) error {
	f.issueCalls++
	f.issuedFor, f.issuedMail = userID, email
	return f.issueErr
}

func (f *fakeVerifier) Verify(ctx context.Context, plaintext string) error {
	f.tokens = append(f.tokens, plaintext)
	return f.err
}

func postVerify(verifier *fakeVerifier, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(verifier, nil).RegisterRoutes(router)
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

func TestHandler_VerifyEmail_AddressTakenMeanwhileIs409(t *testing.T) {
	w := postVerify(&fakeVerifier{err: user.ErrEmailAlreadyTaken}, `{"token":"abc"}`)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestHandler_VerifyEmail_MissingTokenIs400(t *testing.T) {
	verifier := &fakeVerifier{}

	w := postVerify(verifier, `{}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, verifier.tokens)
}

func postResend(verifier *fakeVerifier, accounts *fakeAccounts, authenticated bool) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/")
	protected.Use(auth.RequireAuth(testJWTSecret))
	NewHandler(verifier, accounts).RegisterProtectedRoutes(protected)
	req := httptest.NewRequest(http.MethodPost, "/auth/verify-email/resend", nil)
	if authenticated {
		token, _ := auth.GenerateToken(testJWTSecret, testUserID, time.Hour)
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHandler_Resend_SendsALinkToTheCurrentAddress(t *testing.T) {
	verifier := &fakeVerifier{}
	accounts := &fakeAccounts{user: user.User{ID: testUserID, Email: "me@example.com"}}

	w := postResend(verifier, accounts, true)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, testUserID, verifier.issuedFor)
	assert.Equal(t, "me@example.com", verifier.issuedMail)
}

func TestHandler_Resend_AlreadyVerifiedIs409AndSendsNothing(t *testing.T) {
	verifier := &fakeVerifier{}
	now := time.Now()
	accounts := &fakeAccounts{user: user.User{ID: testUserID, Email: "me@example.com", EmailVerifiedAt: &now}}

	w := postResend(verifier, accounts, true)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Zero(t, verifier.issueCalls)
}

func TestHandler_Resend_RequiresAuthentication(t *testing.T) {
	verifier := &fakeVerifier{}

	w := postResend(verifier, &fakeAccounts{}, false)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Zero(t, verifier.issueCalls)
}

func TestHandler_Resend_SendFailureIs500(t *testing.T) {
	verifier := &fakeVerifier{issueErr: assert.AnError}

	w := postResend(verifier, &fakeAccounts{user: user.User{ID: testUserID}}, true)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
