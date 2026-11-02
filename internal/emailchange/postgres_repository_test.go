//go:build integration

package emailchange

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
		TRUNCATE TABLE tamiyo.email_change_tokens, tamiyo.refresh_tokens, tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage, tamiyo.users
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

func TestPostgresRepository_Consume_ReturnsUserAndNewEmailOnce(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	_, err := repo.Create(context.Background(), ChangeToken{UserID: userID, NewEmail: "new@example.com", TokenHash: "hash-1", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	gotUser, gotEmail, err := repo.Consume(context.Background(), "hash-1")
	require.NoError(t, err)
	assert.Equal(t, userID, gotUser)
	assert.Equal(t, "new@example.com", gotEmail)

	_, _, err = repo.Consume(context.Background(), "hash-1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Consume_RejectsExpiredUsedAndUnknownTokens(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	expired, err := repo.Create(context.Background(), ChangeToken{UserID: userID, NewEmail: "a@example.com", TokenHash: "expired", ExpiresAt: time.Now().Add(-time.Minute)})
	require.NoError(t, err)
	used, err := repo.Create(context.Background(), ChangeToken{UserID: userID, NewEmail: "b@example.com", TokenHash: "used", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE tamiyo.email_change_tokens SET used_at = now() WHERE id = $1`, used.ID)
	require.NoError(t, err)
	require.NotEmpty(t, expired.ID)

	for _, hash := range []string{"expired", "used", "missing"} {
		_, _, err := repo.Consume(context.Background(), hash)
		assert.ErrorIs(t, err, ErrNotFound, hash)
	}
}

func TestPostgresRepository_Consume_ConcurrentCallsYieldExactlyOneSuccess(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	_, err := repo.Create(context.Background(), ChangeToken{UserID: userID, NewEmail: "new@example.com", TokenHash: "hash-1", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	const callers = 8
	var successes atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, _, err := repo.Consume(context.Background(), "hash-1"); err == nil {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), successes.Load())
}
