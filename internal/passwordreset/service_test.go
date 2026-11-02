package passwordreset

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/token"
)

type fakeRepository struct {
	createdTokens []ResetToken
	createErr     error

	consumeUserID string
	consumeErr    error
	consumedHash  string

	invalidatedUsers []string
	invalidateErr    error
}

func (f *fakeRepository) Create(ctx context.Context, t ResetToken) (ResetToken, error) {
	if f.createErr != nil {
		return ResetToken{}, f.createErr
	}
	t.ID = "fake-id"
	f.createdTokens = append(f.createdTokens, t)
	return t, nil
}

func (f *fakeRepository) Consume(ctx context.Context, tokenHash string) (string, error) {
	f.consumedHash = tokenHash
	return f.consumeUserID, f.consumeErr
}

func (f *fakeRepository) InvalidatePending(ctx context.Context, userID string) error {
	f.invalidatedUsers = append(f.invalidatedUsers, userID)
	return f.invalidateErr
}

func TestService_Issue_StoresHashNotPlaintext(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	plaintext, err := service.Issue(context.Background(), "user-1")

	require.NoError(t, err)
	require.Len(t, repo.createdTokens, 1)
	assert.Equal(t, "user-1", repo.createdTokens[0].UserID)
	assert.NotEqual(t, plaintext, repo.createdTokens[0].TokenHash)
	assert.Equal(t, token.Hash(plaintext), repo.createdTokens[0].TokenHash)
}

func TestService_Issue_GeneratesDifferentTokensEachTime(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	first, err := service.Issue(context.Background(), "user-1")
	require.NoError(t, err)
	second, err := service.Issue(context.Background(), "user-1")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
}

func TestService_Issue_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{createErr: assertAnError{}}
	service := NewService(repo, time.Hour)

	_, err := service.Issue(context.Background(), "user-1")

	assert.Error(t, err)
}

func TestService_Consume_ReturnsUserForHashOfPlaintext(t *testing.T) {
	repo := &fakeRepository{consumeUserID: "user-1"}
	service := NewService(repo, time.Hour)

	userID, err := service.Consume(context.Background(), "some-plaintext")

	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.Equal(t, token.Hash("some-plaintext"), repo.consumedHash)
}

func TestService_Consume_MapsNotFoundToErrInvalid(t *testing.T) {
	repo := &fakeRepository{consumeErr: ErrNotFound}
	service := NewService(repo, time.Hour)

	_, err := service.Consume(context.Background(), "unknown")

	assert.ErrorIs(t, err, ErrInvalid)
}

func TestService_Consume_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{consumeErr: assertAnError{}}
	service := NewService(repo, time.Hour)

	_, err := service.Consume(context.Background(), "some-plaintext")

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalid)
}

func TestService_InvalidatePending_DelegatesToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	require.NoError(t, service.InvalidatePending(context.Background(), "user-1"))

	assert.Equal(t, []string{"user-1"}, repo.invalidatedUsers)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "some repository error" }
