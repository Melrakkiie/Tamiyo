package passwordreset

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type fakeResetService struct {
	issueToken string
	issueErr   error

	consumeUserID string
	consumeErr    error
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

func TestHandler_ResetPassword_ReturnsInternalServerErrorForGenericConsumeError(t *testing.T) {
	service := &fakeResetService{consumeErr: assertAnError{}}
	router := setupRouter(service, &fakeUserLookup{}, &fakeSessionRevoker{}, &fakeMailer{})

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"some-token","new_password":"brandnewpassword"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
