package passwordreset

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type fakeResetService struct {
	issueToken string
	issueErr   error

	consumeUserID string
	consumeErr    error

	invalidateErr    error
	invalidateCalled bool
}

func (f *fakeResetService) Issue(ctx context.Context, userID string) (string, error) {
	if f.issueErr != nil {
		return "", f.issueErr
	}
	return f.issueToken, nil
}

func (f *fakeResetService) Consume(ctx context.Context, plaintext string) (string, error) {
	if f.consumeErr != nil {
		return "", f.consumeErr
	}
	return f.consumeUserID, nil
}

func (f *fakeResetService) InvalidatePending(ctx context.Context, userID string) error {
	f.invalidateCalled = true
	return f.invalidateErr
}

// memRepository models single-use and per-user invalidation for the end-to-end handler test.
type memRepository struct{ live map[string]string } // hash -> user

func (m *memRepository) Create(ctx context.Context, t ResetToken) (ResetToken, error) {
	m.live[t.TokenHash] = t.UserID
	return t, nil
}

func (m *memRepository) Consume(ctx context.Context, hash string) (string, error) {
	userID, ok := m.live[hash]
	if !ok {
		return "", ErrNotFound
	}
	delete(m.live, hash)
	return userID, nil
}

func (m *memRepository) InvalidatePending(ctx context.Context, userID string) error {
	for h, u := range m.live {
		if u == userID {
			delete(m.live, h)
		}
	}
	return nil
}

func TestHandler_ResetPassword_VoidsOlderPendingLinks(t *testing.T) {
	service := NewService(&memRepository{live: map[string]string{}}, time.Hour)
	router := setupRouter(service, &fakeUserLookup{}, &fakeSessionRevoker{}, &fakeMailer{})
	first, err := service.Issue(context.Background(), "user-1")
	assert.NoError(t, err)
	second, err := service.Issue(context.Background(), "user-1")
	assert.NoError(t, err)

	post := func(tok string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"`+tok+`","new_password":"new-password-1"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	assert.Equal(t, http.StatusNoContent, post(second))
	assert.Equal(t, http.StatusUnauthorized, post(first))
}

func TestHandler_ResetPassword_ReturnsErrorWhenInvalidatingPendingLinksFails(t *testing.T) {
	service := &fakeResetService{consumeUserID: "user-1", invalidateErr: assertAnError{}}
	router := setupRouter(service, &fakeUserLookup{}, &fakeSessionRevoker{}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"t","new_password":"new-password-1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

type fakeUserLookup struct {
	findIDByEmailID  string
	findIDByEmailErr error

	setPasswordCalledWithUserID string
	setPasswordErr              error
}

func (f *fakeUserLookup) FindIDByEmail(ctx context.Context, email string) (string, error) {
	if f.findIDByEmailErr != nil {
		return "", f.findIDByEmailErr
	}
	return f.findIDByEmailID, nil
}

func (f *fakeUserLookup) SetPassword(ctx context.Context, userID, newPassword string) error {
	f.setPasswordCalledWithUserID = userID
	return f.setPasswordErr
}

type fakeSessionRevoker struct {
	revokeAllCalledWithUserID string
	revokeAllErr              error
}

func (f *fakeSessionRevoker) RevokeAllForUser(ctx context.Context, userID string) error {
	f.revokeAllCalledWithUserID = userID
	return f.revokeAllErr
}

type fakeMailer struct {
	sentToEmail string
	sentToken   string
	sendErr     error
}

func (f *fakeMailer) SendPasswordResetEmail(ctx context.Context, toEmail, resetToken string) error {
	f.sentToEmail = toEmail
	f.sentToken = resetToken
	return f.sendErr
}

func setupRouter(service resetService, users userLookup, tokens sessionRevoker, mailer emailSender) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service, users, tokens, mailer).RegisterRoutes(router)
	return router
}

func TestHandler_ForgotPassword_SendsEmailAndReturnsNoContentForKnownAccount(t *testing.T) {
	service := &fakeResetService{issueToken: "reset-token"}
	users := &fakeUserLookup{findIDByEmailID: "user-1"}
	tokens := &fakeSessionRevoker{}
	mailer := &fakeMailer{}
	router := setupRouter(service, users, tokens, mailer)

	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(`{"email":"alice@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "alice@example.com", mailer.sentToEmail)
	assert.Equal(t, "reset-token", mailer.sentToken)
}

func TestHandler_ForgotPassword_ReturnsNoContentForUnknownAccountWithoutSendingEmail(t *testing.T) {
	service := &fakeResetService{}
	users := &fakeUserLookup{findIDByEmailErr: assertAnError{}}
	tokens := &fakeSessionRevoker{}
	mailer := &fakeMailer{}
	router := setupRouter(service, users, tokens, mailer)

	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(`{"email":"unknown@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "must not reveal whether the account exists")
	assert.Empty(t, mailer.sentToEmail)
}

func TestHandler_ForgotPassword_ReturnsNoContentEvenWhenMailerFails(t *testing.T) {
	service := &fakeResetService{issueToken: "reset-token"}
	users := &fakeUserLookup{findIDByEmailID: "user-1"}
	tokens := &fakeSessionRevoker{}
	mailer := &fakeMailer{sendErr: assertAnError{}}
	router := setupRouter(service, users, tokens, mailer)

	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(`{"email":"alice@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "a mail delivery failure must not be distinguishable from an unknown account")
}

func TestHandler_ForgotPassword_ReturnsBadRequestForInvalidEmail(t *testing.T) {
	router := setupRouter(&fakeResetService{}, &fakeUserLookup{}, &fakeSessionRevoker{}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(`{"email":"not-an-email"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_ResetPassword_UpdatesPasswordAndRevokesSessionsOnSuccess(t *testing.T) {
	service := &fakeResetService{consumeUserID: "user-1"}
	users := &fakeUserLookup{}
	tokens := &fakeSessionRevoker{}
	mailer := &fakeMailer{}
	router := setupRouter(service, users, tokens, mailer)

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"some-token","new_password":"brandnewpassword"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "user-1", users.setPasswordCalledWithUserID)
	assert.Equal(t, "user-1", tokens.revokeAllCalledWithUserID)
}

func TestHandler_ResetPassword_ReturnsUnauthorizedForInvalidToken(t *testing.T) {
	service := &fakeResetService{consumeErr: ErrInvalid}
	router := setupRouter(service, &fakeUserLookup{}, &fakeSessionRevoker{}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"bogus","new_password":"brandnewpassword"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_ResetPassword_ReturnsBadRequestWhenNewPasswordTooShort(t *testing.T) {
	router := setupRouter(&fakeResetService{}, &fakeUserLookup{}, &fakeSessionRevoker{}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"some-token","new_password":"short"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_ResetPassword_EnforcesThe72BytePasswordLimit(t *testing.T) {
	cases := []struct {
		name     string
		password string
		status   int
	}{
		{"72 bytes", strings.Repeat("a", 72), http.StatusNoContent},
		{"73 bytes", strings.Repeat("a", 73), http.StatusBadRequest},
		{"40 accented characters", strings.Repeat("é", 40), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeResetService{consumeUserID: "user-1"}
			router := setupRouter(service, &fakeUserLookup{}, &fakeSessionRevoker{}, &fakeMailer{})

			body := `{"token":"some-token","new_password":"` + tc.password + `"}`
			req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusBadRequest {
				assert.Contains(t, w.Body.String(), "72 bytes")
			}
		})
	}
}

func TestHandler_ResetPassword_ReturnsInternalServerErrorWhenSetPasswordFails(t *testing.T) {
	service := &fakeResetService{consumeUserID: "user-1"}
	users := &fakeUserLookup{setPasswordErr: assertAnError{}}
	router := setupRouter(service, users, &fakeSessionRevoker{}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"some-token","new_password":"brandnewpassword"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_ResetPassword_VoidsPendingLinksEvenWhenSetPasswordFails(t *testing.T) {
	service := &fakeResetService{consumeUserID: "user-1"}
	users := &fakeUserLookup{setPasswordErr: assertAnError{}}
	router := setupRouter(service, users, &fakeSessionRevoker{}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"some-token","new_password":"brandnewpassword"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.True(t, service.invalidateCalled)
}

func TestHandler_ResetPassword_ReturnsInternalServerErrorWhenRevokeAllFails(t *testing.T) {
	service := &fakeResetService{consumeUserID: "user-1"}
	router := setupRouter(service, &fakeUserLookup{}, &fakeSessionRevoker{revokeAllErr: assertAnError{}}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"some-token","new_password":"brandnewpassword"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.True(t, service.invalidateCalled)
}

func TestHandler_ResetPassword_ReturnsInternalServerErrorForGenericConsumeError(t *testing.T) {
	service := &fakeResetService{consumeErr: assertAnError{}}
	router := setupRouter(service, &fakeUserLookup{}, &fakeSessionRevoker{}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"some-token","new_password":"brandnewpassword"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
