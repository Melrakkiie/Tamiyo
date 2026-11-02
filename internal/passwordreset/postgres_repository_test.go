//go:build integration

package passwordreset

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/testdb"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m, &testDB))
}

func getTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	_, err := testDB.Exec(`
		TRUNCATE TABLE tamiyo.password_reset_tokens, tamiyo.refresh_tokens, tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage, tamiyo.users
		RESTART IDENTITY CASCADE
	`)
	require.NoError(t, err)

	return testDB
}

func seedUser(t *testing.T, db *sqlx.DB) string {
	t.Helper()

	var userID string
	err := db.Get(&userID, `
		INSERT INTO tamiyo.users (email, password_hash)
		VALUES ('alice@example.com', 'fake-hash')
		RETURNING id
	`)
	require.NoError(t, err)

	return userID
}

func TestPostgresRepository_Create_InsertsAndReturnsTokenWithID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), ResetToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, userID, created.UserID)
	assert.Nil(t, created.UsedAt)
}

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
	var otherID string
	require.NoError(t, db.Get(&otherID, `INSERT INTO tamiyo.users (email, password_hash) VALUES ('bob@example.com', 'fake-hash') RETURNING id`))
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	expires := time.Now().Add(time.Hour)
	for _, tk := range []ResetToken{
		{UserID: userID, TokenHash: "first", ExpiresAt: expires},
		{UserID: userID, TokenHash: "second", ExpiresAt: expires},
		{UserID: otherID, TokenHash: "other", ExpiresAt: expires},
	} {
		_, err := repo.Create(ctx, tk)
		require.NoError(t, err)
	}

	_, err := repo.Consume(ctx, "second")
	require.NoError(t, err)
	require.NoError(t, repo.InvalidatePending(ctx, userID))

	_, err = repo.Consume(ctx, "first")
	assert.ErrorIs(t, err, ErrNotFound)
	got, err := repo.Consume(ctx, "other")
	require.NoError(t, err)
	assert.Equal(t, otherID, got)
}
