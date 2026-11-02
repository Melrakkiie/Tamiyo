package emailverification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/token"
)

type fakeRepository struct {
	created   []VerificationToken
	createErr error

	consumeUserID, consumeEmail string
	consumeErr                  error
	consumedHash                string
}

func (f *fakeRepository) Create(ctx context.Context, t VerificationToken) (VerificationToken, error) {
	if f.createErr != nil {
		return VerificationToken{}, f.createErr
	}
	f.created = append(f.created, t)
	return t, nil
}

func (f *fakeRepository) Consume(ctx context.Context, tokenHash string) (string, string, error) {
	f.consumedHash = tokenHash
	return f.consumeUserID, f.consumeEmail, f.consumeErr
}

type fakeUsers struct {
	confirmed []string // "userID:email"
	err       error
}

func (f *fakeUsers) ConfirmEmail(ctx context.Context, userID, email string) error {
	f.confirmed = append(f.confirmed, userID+":"+email)
	return f.err
}

type sentMail struct{ to, token string }

type fakeMailer struct {
	sent []sentMail
	err  error
}

func (f *fakeMailer) SendEmailVerification(ctx context.Context, toEmail, tok string) error {
	f.sent = append(f.sent, sentMail{toEmail, tok})
	return f.err
}

var errBoom = errors.New("boom")

func TestService_IssueAndSend_StoresHashAndMailsPlaintext(t *testing.T) {
	repo, mailer := &fakeRepository{}, &fakeMailer{}
	service := NewService(repo, &fakeUsers{}, mailer, time.Hour)

	require.NoError(t, service.IssueAndSend(context.Background(), "user-1", "alice@example.com"))

	require.Len(t, repo.created, 1)
	require.Len(t, mailer.sent, 1)
	assert.Equal(t, "alice@example.com", mailer.sent[0].to)
	assert.Equal(t, "user-1", repo.created[0].UserID)
	assert.Equal(t, "alice@example.com", repo.created[0].Email)
	assert.NotEqual(t, mailer.sent[0].token, repo.created[0].TokenHash)
	assert.Equal(t, token.Hash(mailer.sent[0].token), repo.created[0].TokenHash)
	assert.WithinDuration(t, time.Now().Add(time.Hour), repo.created[0].ExpiresAt, time.Minute)
}

func TestService_IssueAndSend_ReturnsErrorsAndSendsNothingOnStoreFailure(t *testing.T) {
	repo, mailer := &fakeRepository{createErr: errBoom}, &fakeMailer{}
	service := NewService(repo, &fakeUsers{}, mailer, time.Hour)

	err := service.IssueAndSend(context.Background(), "user-1", "alice@example.com")

	assert.ErrorIs(t, err, errBoom)
	assert.Empty(t, mailer.sent)
}

func TestService_IssueAndSend_ReturnsMailerError(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeUsers{}, &fakeMailer{err: errBoom}, time.Hour)

	err := service.IssueAndSend(context.Background(), "user-1", "alice@example.com")

	assert.ErrorIs(t, err, errBoom)
}

func TestService_Verify_ConfirmsTheAddressTheTokenCarries(t *testing.T) {
	repo, users := &fakeRepository{consumeUserID: "user-1", consumeEmail: "new@example.com"}, &fakeUsers{}
	service := NewService(repo, users, &fakeMailer{}, time.Hour)

	require.NoError(t, service.Verify(context.Background(), "plain"))

	assert.Equal(t, token.Hash("plain"), repo.consumedHash)
	assert.Equal(t, []string{"user-1:new@example.com"}, users.confirmed)
}

func TestService_Verify_ConfirmsTheNormalisedAddress(t *testing.T) {
	repo, users := &fakeRepository{consumeUserID: "user-1", consumeEmail: "Bob@Mail.fr"}, &fakeUsers{}
	service := NewService(repo, users, &fakeMailer{}, time.Hour)

	require.NoError(t, service.Verify(context.Background(), "plain"))

	assert.Equal(t, []string{"user-1:bob@mail.fr"}, users.confirmed)
}

func TestService_IssueAndSend_StoresAndMailsTheNormalisedAddress(t *testing.T) {
	repo, mailer := &fakeRepository{}, &fakeMailer{}
	service := NewService(repo, &fakeUsers{}, mailer, time.Hour)

	require.NoError(t, service.IssueAndSend(context.Background(), "user-1", " Bob@Mail.fr "))

	assert.Equal(t, "bob@mail.fr", repo.created[0].Email)
	assert.Equal(t, "bob@mail.fr", mailer.sent[0].to)
}

func TestService_Verify_MapsNotFoundToErrInvalid(t *testing.T) {
	users := &fakeUsers{}
	service := NewService(&fakeRepository{consumeErr: ErrNotFound}, users, &fakeMailer{}, time.Hour)

	err := service.Verify(context.Background(), "unknown")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Empty(t, users.confirmed)
}

func TestService_Verify_PropagatesUserError(t *testing.T) {
	service := NewService(&fakeRepository{consumeUserID: "user-1"}, &fakeUsers{err: errBoom}, &fakeMailer{}, time.Hour)

	assert.ErrorIs(t, service.Verify(context.Background(), "plain"), errBoom)
}
