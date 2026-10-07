package emailchange

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/user"
)

const testJWTSecret = "test-secret"
const testUserID = "11111111-1111-1111-1111-111111111111"

type fakeChangeService struct {
	issuedToken    string
	issueErr       error
	issuedForUser  string
	issuedForEmail string

	consumeUserID  string
	consumeEmail   string
	consumeErr     error
	consumedTokens []string
}

func (f *fakeChangeService) Issue(ctx context.Context, userID, newEmail string) (string, error) {
	f.issuedForUser = userID
	f.issuedForEmail = newEmail
	return f.issuedToken, f.issueErr
}

func (f *fakeChangeService) Consume(ctx context.Context, plaintext string) (string, string, error) {
	f.consumedTokens = append(f.consumedTokens, plaintext)
	return f.consumeUserID, f.consumeEmail, f.consumeErr
}

type fakeAccounts struct {
	currentEmail     string
	checkPasswordErr error

	taken    bool
	takenErr error

	oldEmail       string
	changeEmailErr error
	changedUserID  string
	changedTo      string
}

func (f *fakeAccounts) CheckPassword(ctx context.Context, userID, password string) (string, error) {
	return f.currentEmail, f.checkPasswordErr
}

func (f *fakeAccounts) EmailTaken(ctx context.Context, email string) (bool, error) {
	return f.taken, f.takenErr
}

func (f *fakeAccounts) ChangeEmail(ctx context.Context, userID, newEmail string) (string, error) {
	f.changedUserID = userID
	f.changedTo = newEmail
	return f.oldEmail, f.changeEmailErr
}

type sentConfirmation struct {
	to       string
	newEmail string
	token    string
}

type fakeMailer struct {
	confirmations []sentConfirmation
	confirmErr    error
	notices       []string
	noticeErr     error
}

func (f *fakeMailer) SendEmailChangeConfirmation(ctx context.Context, toEmail, newEmail, token string) error {
	f.confirmations = append(f.confirmations, sentConfirmation{to: toEmail, newEmail: newEmail, token: token})
	return f.confirmErr
}

func (f *fakeMailer) SendEmailChangedNotice(ctx context.Context, toEmail string) error {
	f.notices = append(f.notices, toEmail)
	return f.noticeErr
}

func setupRouter(service changeService, accounts userAccounts, mailer emailSender) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(service, accounts, mailer)
	handler.RegisterRoutes(router)
	protected := router.Group("/")
	protected.Use(auth.RequireAuth(testJWTSecret))
	handler.RegisterProtectedRoutes(protected)
	return router
}

func post(router *gin.Engine, path, body string, authenticated bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if authenticated {
		token, _ := auth.GenerateToken(testJWTSecret, testUserID, time.Hour)
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

const requestBody = `{"current_password": "supersecret", "new_email": "new@example.com"}`

func TestHandler_RequestEmailChange_EmailsAConfirmationTokenToTheCurrentAddress(t *testing.T) {
	service := &fakeChangeService{issuedToken: "change-token"}
	mailer := &fakeMailer{}
	router := setupRouter(service, &fakeAccounts{currentEmail: "alice@example.com"}, mailer)

	w := post(router, "/auth/email", requestBody, true)

	require.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, testUserID, service.issuedForUser)
	assert.Equal(t, "new@example.com", service.issuedForEmail)
	assert.Equal(t, []sentConfirmation{{to: "alice@example.com", newEmail: "new@example.com", token: "change-token"}}, mailer.confirmations)
}

func TestHandler_RequestEmailChange_RequiresAuthentication(t *testing.T) {
	router := setupRouter(&fakeChangeService{}, &fakeAccounts{}, &fakeMailer{})

	w := post(router, "/auth/email", requestBody, false)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_RequestEmailChange_RejectsAnIncorrectPassword(t *testing.T) {
	service := &fakeChangeService{}
	mailer := &fakeMailer{}
	router := setupRouter(service, &fakeAccounts{checkPasswordErr: user.ErrIncorrectPassword}, mailer)

	w := post(router, "/auth/email", requestBody, true)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "incorrect current password")
	assert.Empty(t, service.issuedForUser)
	assert.Empty(t, mailer.confirmations)
}

func TestHandler_RequestEmailChange_RejectsAnInvalidEmail(t *testing.T) {
	router := setupRouter(&fakeChangeService{}, &fakeAccounts{}, &fakeMailer{})

	w := post(router, "/auth/email", `{"current_password": "supersecret", "new_email": "not-an-email"}`, true)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_RequestEmailChange_RejectsTheCurrentEmail(t *testing.T) {
	service := &fakeChangeService{}
	router := setupRouter(service, &fakeAccounts{currentEmail: "New@Example.com"}, &fakeMailer{})

	w := post(router, "/auth/email", requestBody, true)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, service.issuedForUser)
}

func TestHandler_RequestEmailChange_RejectsAnEmailAlreadyRegistered(t *testing.T) {
	service := &fakeChangeService{}
	router := setupRouter(service, &fakeAccounts{currentEmail: "alice@example.com", taken: true}, &fakeMailer{})

	w := post(router, "/auth/email", requestBody, true)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Empty(t, service.issuedForUser)
}

func TestHandler_RequestEmailChange_ReturnsInternalServerErrorWhenTheEmailCannotBeSent(t *testing.T) {
	router := setupRouter(&fakeChangeService{issuedToken: "change-token"}, &fakeAccounts{currentEmail: "alice@example.com"}, &fakeMailer{confirmErr: assert.AnError})

	w := post(router, "/auth/email", requestBody, true)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_ConfirmEmailChange_ChangesTheEmailAndNotifiesTheNewAddress(t *testing.T) {
	service := &fakeChangeService{consumeUserID: testUserID, consumeEmail: "new@example.com"}
	accounts := &fakeAccounts{oldEmail: "alice@example.com"}
	mailer := &fakeMailer{}
	router := setupRouter(service, accounts, mailer)

	w := post(router, "/auth/confirm-email", `{"token": "change-token"}`, false)

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, []string{"change-token"}, service.consumedTokens)
	assert.Equal(t, testUserID, accounts.changedUserID)
	assert.Equal(t, "new@example.com", accounts.changedTo)
	assert.Equal(t, []string{"new@example.com"}, mailer.notices)
}

func TestHandler_ConfirmEmailChange_SucceedsEvenWhenTheNoticeCannotBeSent(t *testing.T) {
	service := &fakeChangeService{consumeUserID: testUserID, consumeEmail: "new@example.com"}
	router := setupRouter(service, &fakeAccounts{oldEmail: "alice@example.com"}, &fakeMailer{noticeErr: assert.AnError})

	w := post(router, "/auth/confirm-email", `{"token": "change-token"}`, false)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandler_ConfirmEmailChange_RejectsAnInvalidToken(t *testing.T) {
	accounts := &fakeAccounts{}
	router := setupRouter(&fakeChangeService{consumeErr: ErrInvalid}, accounts, &fakeMailer{})

	w := post(router, "/auth/confirm-email", `{"token": "bad"}`, false)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Empty(t, accounts.changedUserID)
}

func TestHandler_ConfirmEmailChange_ReturnsConflictWhenTheEmailWasTakenMeanwhile(t *testing.T) {
	service := &fakeChangeService{consumeUserID: testUserID, consumeEmail: "new@example.com"}
	mailer := &fakeMailer{}
	router := setupRouter(service, &fakeAccounts{changeEmailErr: user.ErrEmailAlreadyTaken}, mailer)

	w := post(router, "/auth/confirm-email", `{"token": "change-token"}`, false)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Empty(t, mailer.notices)
}

func TestHandler_ConfirmEmailChange_RequiresAToken(t *testing.T) {
	router := setupRouter(&fakeChangeService{}, &fakeAccounts{}, &fakeMailer{})

	w := post(router, "/auth/confirm-email", `{}`, false)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
