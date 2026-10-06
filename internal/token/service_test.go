package token

import (
	"context"
	"errors"
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

	byID map[string]RefreshToken

	revokedIDs []string
	revokeErr  error

	replaced   map[string]string
	replaceErr error

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

func (f *fakeRepository) FindByID(ctx context.Context, id string) (RefreshToken, error) {
	t, ok := f.byID[id]
	if !ok {
		return RefreshToken{}, ErrNotFound
	}
	return t, nil
}

func (f *fakeRepository) Replace(ctx context.Context, id string, successorID string) error {
	if f.replaceErr != nil {
		return f.replaceErr
	}
	if f.replaced == nil {
		f.replaced = map[string]string{}
	}
	f.replaced[id] = successorID
	return nil
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

func TestService_IssueRefreshToken_PropagatesRepositoryCreateError(t *testing.T) {
	repo := &fakeRepository{createErr: errors.New("boom")}
	service := NewService(repo, time.Hour)

	plaintext, err := service.IssueRefreshToken(context.Background(), "user-1")

	require.Error(t, err)
	assert.Empty(t, plaintext)
}

func TestService_Rotate_IssuesNewTokenAndReplacesOldOne(t *testing.T) {
	repo := &fakeRepository{
		findByHashResult: RefreshToken{ID: "old-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
	}
	service := NewService(repo, time.Hour)

	userID, newPlaintext, err := service.Rotate(context.Background(), "old-plaintext")

	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.NotEmpty(t, newPlaintext)
	require.Len(t, repo.createdTokens, 1)
	assert.Equal(t, hash(newPlaintext), repo.createdTokens[0].TokenHash)
	assert.Equal(t, "fake-id", repo.replaced["old-id"])
}

func TestService_Rotate_PropagatesReplaceError(t *testing.T) {
	repo := &fakeRepository{
		findByHashResult: RefreshToken{ID: "old-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		replaceErr:       errors.New("boom"),
	}
	service := NewService(repo, time.Hour)

	_, newPlaintext, err := service.Rotate(context.Background(), "old-plaintext")

	require.Error(t, err)
	assert.Empty(t, newPlaintext)
}

func TestService_Rotate_AcceptsJustRotatedTokenAndRotatesItsSuccessor(t *testing.T) {
	rotatedAt := time.Now().Add(-2 * time.Second)
	successorID := "successor-id"
	repo := &fakeRepository{
		findByHashResult: RefreshToken{
			ID:         "old-id",
			UserID:     "user-1",
			ExpiresAt:  time.Now().Add(time.Hour),
			RevokedAt:  &rotatedAt,
			ReplacedBy: &successorID,
		},
		byID: map[string]RefreshToken{
			"successor-id": {ID: "successor-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	service := NewService(repo, time.Hour)

	userID, newPlaintext, err := service.Rotate(context.Background(), "old-plaintext")

	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.NotEmpty(t, newPlaintext)
	assert.Equal(t, "fake-id", repo.replaced["successor-id"])
	assert.Empty(t, repo.revokeAllForUserCalledWith)
}

func TestService_Rotate_FollowsAChainOfJustRotatedTokens(t *testing.T) {
	firstRotation := time.Now().Add(-3 * time.Second)
	secondRotation := time.Now().Add(-1 * time.Second)
	middleID, lastID := "middle-id", "last-id"
	repo := &fakeRepository{
		findByHashResult: RefreshToken{
			ID:         "old-id",
			UserID:     "user-1",
			ExpiresAt:  time.Now().Add(time.Hour),
			RevokedAt:  &firstRotation,
			ReplacedBy: &middleID,
		},
		byID: map[string]RefreshToken{
			"middle-id": {ID: "middle-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour), RevokedAt: &secondRotation, ReplacedBy: &lastID},
			"last-id":   {ID: "last-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	service := NewService(repo, time.Hour)

	_, _, err := service.Rotate(context.Background(), "old-plaintext")

	require.NoError(t, err)
	assert.Equal(t, "fake-id", repo.replaced["last-id"])
	assert.Empty(t, repo.revokeAllForUserCalledWith)
}

func TestService_Rotate_TreatsReuseAfterTheGracePeriodAsTheft(t *testing.T) {
	rotatedAt := time.Now().Add(-time.Minute)
	successorID := "successor-id"
	repo := &fakeRepository{
		findByHashResult: RefreshToken{
			ID:         "old-id",
			UserID:     "user-1",
			ExpiresAt:  time.Now().Add(time.Hour),
			RevokedAt:  &rotatedAt,
			ReplacedBy: &successorID,
		},
		byID: map[string]RefreshToken{
			"successor-id": {ID: "successor-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	service := NewService(repo, time.Hour)

	_, _, err := service.Rotate(context.Background(), "old-plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Equal(t, "user-1", repo.revokeAllForUserCalledWith)
	assert.Empty(t, repo.replaced)
}

func TestService_Rotate_TreatsReuseAsTheftWhenTheSuccessorWasRevoked(t *testing.T) {
	rotatedAt := time.Now().Add(-2 * time.Second)
	revokedAt := time.Now().Add(-1 * time.Second)
	successorID := "successor-id"
	repo := &fakeRepository{
		findByHashResult: RefreshToken{
			ID:         "old-id",
			UserID:     "user-1",
			ExpiresAt:  time.Now().Add(time.Hour),
			RevokedAt:  &rotatedAt,
			ReplacedBy: &successorID,
		},
		byID: map[string]RefreshToken{
			"successor-id": {ID: "successor-id", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour), RevokedAt: &revokedAt},
		},
	}
	service := NewService(repo, time.Hour)

	_, _, err := service.Rotate(context.Background(), "old-plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Equal(t, "user-1", repo.revokeAllForUserCalledWith)
}

func TestService_Rotate_TreatsReuseOfAJustLoggedOutTokenAsTheft(t *testing.T) {
	revokedAt := time.Now().Add(-1 * time.Second)
	repo := &fakeRepository{
		findByHashResult: RefreshToken{
			ID:        "old-id",
			UserID:    "user-1",
			ExpiresAt: time.Now().Add(time.Hour),
			RevokedAt: &revokedAt,
		},
	}
	service := NewService(repo, time.Hour)

	_, _, err := service.Rotate(context.Background(), "logged-out-plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Equal(t, "user-1", repo.revokeAllForUserCalledWith)
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
