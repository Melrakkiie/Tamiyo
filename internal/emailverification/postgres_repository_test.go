//go:build integration

package emailverification

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
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

	_, err := testDB.Exec(`TRUNCATE TABLE tamiyo.email_verification_tokens, tamiyo.users RESTART IDENTITY CASCADE`)
	require.NoError(t, err)

	return testDB
}

func seedUser(t *testing.T, db *sqlx.DB) string {
	t.Helper()

	return seedUserWithEmail(t, db, "alice@example.com")
}

func seedUserWithEmail(t *testing.T, db *sqlx.DB, email string) string {
	t.Helper()

	var userID string
	require.NoError(t, db.Get(&userID, `INSERT INTO tamiyo.users (email, password_hash) VALUES ($1, 'fake-hash') RETURNING id`, email))

	return userID
}

func TestPostgresRepository_Consume_ReturnsUserOnceThenErrNotFound(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	created, err := repo.Create(context.Background(), VerificationToken{UserID: userID, Email: "new@example.com", TokenHash: "h", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)

	got, email, err := repo.Consume(context.Background(), "h")
	require.NoError(t, err)
	assert.Equal(t, userID, got)
	assert.Equal(t, "new@example.com", email)

	_, _, err = repo.Consume(context.Background(), "h")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMigration_ExistingUsersAreVerifiedAfterwards(t *testing.T) {
	db := getTestDB(t)
	// Roll back to just before 00057 so later migrations don't shift which one is undone.
	require.NoError(t, goose.DownTo(db.DB, ".", 56))
	t.Cleanup(func() { require.NoError(t, goose.Up(db.DB, ".")) })
	var userID string
	require.NoError(t, db.Get(&userID, `INSERT INTO tamiyo.users (email, password_hash) VALUES ('old@example.com', 'fake-hash') RETURNING id`))

	require.NoError(t, goose.Up(db.DB, "."))

	var verified bool
	require.NoError(t, db.Get(&verified, `SELECT email_verified_at IS NOT NULL FROM tamiyo.users WHERE id = $1`, userID))
	assert.True(t, verified)
}

func TestPostgresRepository_Consume_RejectsExpiredAndUnknown(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)
	_, err := repo.Create(context.Background(), VerificationToken{UserID: userID, Email: "alice@example.com", TokenHash: "old", ExpiresAt: time.Now().Add(-time.Minute)})
	require.NoError(t, err)

	_, _, err = repo.Consume(context.Background(), "old")
	assert.ErrorIs(t, err, ErrNotFound)
	_, _, err = repo.Consume(context.Background(), "unknown")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Create_DeletesOnlyTheUsersExpiredTokens(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	otherID := seedUserWithEmail(t, db, "bob@example.com")
	_, err := db.Exec(`
		INSERT INTO tamiyo.email_verification_tokens (user_id, email, token_hash, expires_at, used_at) VALUES
		($1, 'a@example.com', 'expired', now() - interval '1 minute', NULL),
		($1, 'b@example.com', 'fresh', now() + interval '1 hour', NULL),
		($1, 'c@example.com', 'used', now() + interval '1 hour', now()),
		($2, 'd@example.com', 'other-expired', now() - interval '1 minute', NULL)
	`, userID, otherID)
	require.NoError(t, err)
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	_, err = repo.Create(ctx, VerificationToken{UserID: userID, Email: "e@example.com", TokenHash: "new", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	var hashes []string
	require.NoError(t, db.Select(&hashes, `SELECT token_hash FROM tamiyo.email_verification_tokens ORDER BY token_hash`))
	assert.Equal(t, []string{"fresh", "new", "other-expired", "used"}, hashes)
}
