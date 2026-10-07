package emailchange

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	createdTokens []ChangeToken
	createErr     error

	findByHashResult ChangeToken
	findByHashErr    error

	markUsedIDs []string
	markUsedErr error
}

func (f *fakeRepository) Create(ctx context.Context, t ChangeToken) (ChangeToken, error) {
	if f.createErr != nil {
		return ChangeToken{}, f.createErr
	}
	t.ID = "fake-id"
	f.createdTokens = append(f.createdTokens, t)
	return t, nil
}

func (f *fakeRepository) FindByHash(ctx context.Context, tokenHash string) (ChangeToken, error) {
	if f.findByHashErr != nil {
		return ChangeToken{}, f.findByHashErr
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

func TestService_Issue_StoresTheNewEmailAndTheHashNotThePlaintext(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, time.Hour)

	plaintext, err := service.Issue(context.Background(), "user-1", "new@example.com")

	require.NoError(t, err)
	require.Len(t, repo.createdTokens, 1)
	created := repo.createdTokens[0]
	assert.Equal(t, "user-1", created.UserID)
	assert.Equal(t, "new@example.com", created.NewEmail)
	assert.Equal(t, hash(plaintext), created.TokenHash)
	assert.NotEqual(t, plaintext, created.TokenHash)
	assert.WithinDuration(t, time.Now().Add(time.Hour), created.ExpiresAt, 5*time.Second)
}

func TestService_Issue_PropagatesCreateError(t *testing.T) {
	service := NewService(&fakeRepository{createErr: assert.AnError}, time.Hour)

	_, err := service.Issue(context.Background(), "user-1", "new@example.com")

	assert.ErrorIs(t, err, assert.AnError)
}

func TestService_Consume_ReturnsUserAndNewEmailAndMarksTheTokenUsed(t *testing.T) {
	repo := &fakeRepository{findByHashResult: ChangeToken{ID: "token-1", UserID: "user-1", NewEmail: "new@example.com", ExpiresAt: time.Now().Add(time.Hour)}}
	service := NewService(repo, time.Hour)

	userID, newEmail, err := service.Consume(context.Background(), "plaintext")

	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.Equal(t, "new@example.com", newEmail)
	assert.Equal(t, []string{"token-1"}, repo.markUsedIDs)
}

func TestService_Consume_RejectsUnknownToken(t *testing.T) {
	service := NewService(&fakeRepository{findByHashErr: ErrNotFound}, time.Hour)

	_, _, err := service.Consume(context.Background(), "plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
}

func TestService_Consume_RejectsUsedToken(t *testing.T) {
	used := time.Now().Add(-time.Minute)
	repo := &fakeRepository{findByHashResult: ChangeToken{ID: "token-1", ExpiresAt: time.Now().Add(time.Hour), UsedAt: &used}}
	service := NewService(repo, time.Hour)

	_, _, err := service.Consume(context.Background(), "plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Empty(t, repo.markUsedIDs)
}

func TestService_Consume_RejectsExpiredToken(t *testing.T) {
	repo := &fakeRepository{findByHashResult: ChangeToken{ID: "token-1", ExpiresAt: time.Now().Add(-time.Minute)}}
	service := NewService(repo, time.Hour)

	_, _, err := service.Consume(context.Background(), "plaintext")

	assert.ErrorIs(t, err, ErrInvalid)
	assert.Empty(t, repo.markUsedIDs)
}

func TestService_Consume_PropagatesOtherLookupErrors(t *testing.T) {
	service := NewService(&fakeRepository{findByHashErr: assert.AnError}, time.Hour)

	_, _, err := service.Consume(context.Background(), "plaintext")

	assert.ErrorIs(t, err, assert.AnError)
}
