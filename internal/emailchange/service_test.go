package emailchange

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/token"
)

type fakeRepository struct {
	createdTokens []ChangeToken
	createErr     error

	consumeUserID   string
	consumeNewEmail string
	consumeErr      error
}

func (f *fakeRepository) Create(ctx context.Context, t ChangeToken) (ChangeToken, error) {
	if f.createErr != nil {
		return ChangeToken{}, f.createErr
	}
	t.ID = "fake-id"
	f.createdTokens = append(f.createdTokens, t)
	return t, nil
}

func (f *fakeRepository) Consume(ctx context.Context, tokenHash string) (string, string, error) {
	return f.consumeUserID, f.consumeNewEmail, f.consumeErr
}

func TestService_Issue_StoresTheNewEmailAndTheHashNotThePlaintext(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	plaintext, err := service.Issue(context.Background(), "user-1", "new@example.com")

	require.NoError(t, err)
	require.Len(t, repo.createdTokens, 1)
	created := repo.createdTokens[0]
	assert.Equal(t, "user-1", created.UserID)
	assert.Equal(t, "new@example.com", created.NewEmail)
	assert.Equal(t, token.Hash(plaintext), created.TokenHash)
	assert.NotEqual(t, plaintext, created.TokenHash)
	assert.WithinDuration(t, time.Now().Add(time.Hour), created.ExpiresAt, 5*time.Second)
}

func TestService_Issue_PropagatesCreateError(t *testing.T) {
	service := NewService(&fakeRepository{createErr: assert.AnError}, time.Hour)

	_, err := service.Issue(context.Background(), "user-1", "new@example.com")

	assert.ErrorIs(t, err, assert.AnError)
}

func TestService_Consume_ReturnsUserAndNewEmail(t *testing.T) {
	repo := &fakeRepository{consumeUserID: "user-1", consumeNewEmail: "new@example.com"}
	service := NewService(repo, time.Hour)

	userID, newEmail, err := service.Consume(context.Background(), "plaintext")

	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.Equal(t, "new@example.com", newEmail)
}

func TestService_Consume_MapsNoMatchingTokenToErrInvalid(t *testing.T) {
	service := NewService(&fakeRepository{consumeErr: ErrNotFound}, time.Hour)

	_, _, err := service.Consume(context.Background(), "plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
}

func TestService_Consume_PropagatesOtherLookupErrors(t *testing.T) {
	service := NewService(&fakeRepository{consumeErr: assert.AnError}, time.Hour)

	_, _, err := service.Consume(context.Background(), "plaintext")

	assert.ErrorIs(t, err, assert.AnError)
}
