package token

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	createdTokens []RefreshToken
	createErr     error

	findByHashResult RefreshToken
	findByHashErr    error

	revokedIDs []string
	revokeErr  error

	revokeAllForUserCalledWith string
	revokeAllErr               error
}

func (f *fakeRepository) Create(ctx context.Context, t RefreshToken) (RefreshToken, error) {
	if f.createErr != nil {
		return RefreshToken{}, f.createErr
	}
	t.ID = "fake-id"
	f.createdTokens = append(f.createdTokens, t)
	return t, nil
}

func (f *fakeRepository) FindByHash(ctx context.Context, tokenHash string) (RefreshToken, error) {
	if f.findByHashErr != nil {
		return RefreshToken{}, f.findByHashErr
	}
	return f.findByHashResult, nil
}

func (f *fakeRepository) Revoke(ctx context.Context, id string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	f.revokedIDs = append(f.revokedIDs, id)
	return nil
}

func (f *fakeRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	f.revokeAllForUserCalledWith = userID
	return f.revokeAllErr
}

func TestService_IssueRefreshToken_StoresHashNotPlaintext(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	plaintext, err := service.IssueRefreshToken(context.Background(), "user-1")

	require.NoError(t, err)
	require.Len(t, repo.createdTokens, 1)
	assert.Equal(t, "user-1", repo.createdTokens[0].UserID)
	assert.NotEqual(t, plaintext, repo.createdTokens[0].TokenHash)
	assert.Equal(t, hash(plaintext), repo.createdTokens[0].TokenHash)
}

func TestService_IssueRefreshToken_GeneratesDifferentTokensEachTime(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	first, err := service.IssueRefreshToken(context.Background(), "user-1")
	require.NoError(t, err)
	second, err := service.IssueRefreshToken(context.Background(), "user-1")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
}

func TestService_Rotate_IssuesNewTokenAndRevokesOldOne(t *testing.T) {
	repo := &fakeRepository{
		findByHashResult: RefreshToken{ID: "old-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
	}
	service := NewService(repo, time.Hour)

	userID, newPlaintext, err := service.Rotate(context.Background(), "old-plaintext")

	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.NotEmpty(t, newPlaintext)
	assert.Contains(t, repo.revokedIDs, "old-id")
}

func TestService_Rotate_FailsWithUnknownToken(t *testing.T) {
	repo := &fakeRepository{findByHashErr: ErrNotFound}
	service := NewService(repo, time.Hour)

	_, _, err := service.Rotate(context.Background(), "unknown")

	assert.ErrorIs(t, err, ErrInvalid)
}

func TestService_Rotate_FailsWithExpiredToken(t *testing.T) {
	repo := &fakeRepository{
		findByHashResult: RefreshToken{ID: "old-id", UserID: "user-1", ExpiresAt: time.Now().Add(-time.Minute)},
	}
	service := NewService(repo, time.Hour)

	_, _, err := service.Rotate(context.Background(), "expired-plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
}

func TestService_Rotate_DetectsReuseAndRevokesEverySessionForTheUser(t *testing.T) {
	revokedAt := time.Now().Add(-time.Minute)
	repo := &fakeRepository{
		findByHashResult: RefreshToken{
			ID:        "old-id",
			UserID:    "user-1",
			ExpiresAt: time.Now().Add(time.Hour),
			RevokedAt: &revokedAt,
		},
	}
	service := NewService(repo, time.Hour)

	_, _, err := service.Rotate(context.Background(), "already-used-plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Equal(t, "user-1", repo.revokeAllForUserCalledWith)
}

func TestService_Revoke_RevokesTheMatchingToken(t *testing.T) {
	repo := &fakeRepository{findByHashResult: RefreshToken{ID: "token-id", UserID: "user-1"}}
	service := NewService(repo, time.Hour)

	err := service.Revoke(context.Background(), "some-plaintext")

	require.NoError(t, err)
	assert.Contains(t, repo.revokedIDs, "token-id")
}

func TestService_Revoke_IsIdempotentForUnknownToken(t *testing.T) {
	repo := &fakeRepository{findByHashErr: ErrNotFound}
	service := NewService(repo, time.Hour)

	err := service.Revoke(context.Background(), "unknown-plaintext")

	assert.NoError(t, err)
}

func TestService_RevokeAllForUser_DelegatesToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	err := service.RevokeAllForUser(context.Background(), "user-1")

	require.NoError(t, err)
	assert.Equal(t, "user-1", repo.revokeAllForUserCalledWith)
}
