//go:build integration

package emailchange

import (
	"context"
	"os"
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

func TestPostgresRepository_CreateThenFindByHash_RoundTripsTheToken(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	expires := time.Now().Add(time.Hour).Truncate(time.Microsecond)

	created, err := repo.Create(context.Background(), ChangeToken{UserID: userID, NewEmail: "new@example.com", TokenHash: "hash-1", ExpiresAt: expires})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)

	found, err := repo.FindByHash(context.Background(), "hash-1")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, userID, found.UserID)
	assert.Equal(t, "new@example.com", found.NewEmail)
	assert.True(t, found.ExpiresAt.Equal(expires))
	assert.Nil(t, found.UsedAt)
}

func TestPostgresRepository_FindByHash_ReturnsErrNotFound(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByHash(context.Background(), "missing")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_MarkUsed_SetsUsedAt(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), ChangeToken{UserID: userID, NewEmail: "new@example.com", TokenHash: "hash-1", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	require.NoError(t, repo.MarkUsed(context.Background(), created.ID))

	found, err := repo.FindByHash(context.Background(), "hash-1")
	require.NoError(t, err)
	assert.NotNil(t, found.UsedAt)
}
