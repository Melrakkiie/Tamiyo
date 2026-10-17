//go:build integration

package passwordreset

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

func TestPostgresRepository_FindByHash_ReturnsToken(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), ResetToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	result, err := repo.FindByHash(context.Background(), "fake-hash")

	require.NoError(t, err)
	assert.Equal(t, created.ID, result.ID)
}

func TestPostgresRepository_FindByHash_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByHash(context.Background(), "unknown-hash")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_MarkUsed_SetsUsedAt(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), ResetToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	err = repo.MarkUsed(context.Background(), created.ID)
	require.NoError(t, err)

	result, err := repo.FindByHash(context.Background(), "fake-hash")
	require.NoError(t, err)
	assert.NotNil(t, result.UsedAt)
}
