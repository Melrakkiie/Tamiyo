//go:build integration

package passwordreset

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepository_Consume_ReturnsUserOnceThenErrNotFound(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	_, err := repo.Create(context.Background(), ResetToken{UserID: userID, TokenHash: "h", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	got, err := repo.Consume(context.Background(), "h")
	require.NoError(t, err)
	assert.Equal(t, userID, got)

	_, err = repo.Consume(context.Background(), "h")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Consume_RejectsExpiredAndUnknown(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	_, err := repo.Create(context.Background(), ResetToken{UserID: userID, TokenHash: "old", ExpiresAt: time.Now().Add(-time.Minute)})
	require.NoError(t, err)

	_, err = repo.Consume(context.Background(), "old")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = repo.Consume(context.Background(), "unknown")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Consume_ConcurrentCallsYieldOneSuccess(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	_, err := repo.Create(context.Background(), ResetToken{UserID: userID, TokenHash: "h", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	var wins atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := repo.Consume(context.Background(), "h"); err == nil {
				wins.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, wins.Load())
}

func TestPostgresRepository_InvalidatePending_VoidsOtherLinksOfTheUser(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	_, err := repo.Create(context.Background(), ResetToken{UserID: userID, TokenHash: "a", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	require.NoError(t, repo.InvalidatePending(context.Background(), userID))

	_, err = repo.Consume(context.Background(), "a")
	assert.ErrorIs(t, err, ErrNotFound)
}
