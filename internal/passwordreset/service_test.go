package passwordreset

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	createdTokens []ResetToken
	createErr     error

	findByHashResult ResetToken
	findByHashErr    error

	markUsedIDs []string
	markUsedErr error
}

func (f *fakeRepository) Create(ctx context.Context, t ResetToken) (ResetToken, error) {
	if f.createErr != nil {
		return ResetToken{}, f.createErr
	}
	t.ID = "fake-id"
	f.createdTokens = append(f.createdTokens, t)
	return t, nil
}

func (f *fakeRepository) FindByHash(ctx context.Context, tokenHash string) (ResetToken, error) {
	if f.findByHashErr != nil {
		return ResetToken{}, f.findByHashErr
	}
	return f.findByHashResult, nil
}

func (f *fakeRepository) MarkUsed(ctx context.Context, id string) error {
	if f.markUsedErr != nil {
		return f.markUsedErr
	}
	f.markUsedIDs = append(f.markUsedIDs, id)
	return nil
}

func TestService_Issue_StoresHashNotPlaintext(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	plaintext, err := service.Issue(context.Background(), "user-1")

	require.NoError(t, err)
	require.Len(t, repo.createdTokens, 1)
	assert.Equal(t, "user-1", repo.createdTokens[0].UserID)
	assert.NotEqual(t, plaintext, repo.createdTokens[0].TokenHash)
	assert.Equal(t, hash(plaintext), repo.createdTokens[0].TokenHash)
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

func TestService_Consume_SucceedsAndMarksTokenUsed(t *testing.T) {
	repo := &fakeRepository{
		findByHashResult: ResetToken{ID: "token-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
	}
	service := NewService(repo, time.Hour)

	userID, err := service.Consume(context.Background(), "some-plaintext")

	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.Contains(t, repo.markUsedIDs, "token-id")
}

func TestService_Consume_FailsWithUnknownToken(t *testing.T) {
	repo := &fakeRepository{findByHashErr: ErrNotFound}
	service := NewService(repo, time.Hour)

	_, err := service.Consume(context.Background(), "unknown")

	assert.ErrorIs(t, err, ErrInvalid)
}

func TestService_Consume_FailsWithExpiredToken(t *testing.T) {
	repo := &fakeRepository{
		findByHashResult: ResetToken{ID: "token-id", UserID: "user-1", ExpiresAt: time.Now().Add(-time.Minute)},
	}
	service := NewService(repo, time.Hour)

	_, err := service.Consume(context.Background(), "expired-plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Empty(t, repo.markUsedIDs)
}

func TestService_Consume_FailsWithAlreadyUsedToken(t *testing.T) {
	usedAt := time.Now().Add(-time.Minute)
	repo := &fakeRepository{
		findByHashResult: ResetToken{
			ID:        "token-id",
			UserID:    "user-1",
			ExpiresAt: time.Now().Add(time.Hour),
			UsedAt:    &usedAt,
		},
	}
	service := NewService(repo, time.Hour)

	_, err := service.Consume(context.Background(), "already-used-plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Empty(t, repo.markUsedIDs)
}

func TestService_Consume_PropagatesMarkUsedError(t *testing.T) {
	repo := &fakeRepository{
		findByHashResult: ResetToken{ID: "token-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		markUsedErr:      assertAnError{},
	}
	service := NewService(repo, time.Hour)

	_, err := service.Consume(context.Background(), "some-plaintext")

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalid)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "some repository error" }
