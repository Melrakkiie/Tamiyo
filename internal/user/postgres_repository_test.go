//go:build integration

package user

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
		TRUNCATE TABLE tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage, tamiyo.users
		RESTART IDENTITY CASCADE
	`)
	require.NoError(t, err)

	return testDB
}

func TestPostgresRepository_Create_InsertsAndReturnsUserWithID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	newUser := User{
		Email:        "alice@example.com",
		PasswordHash: "fake-hash",
	}

	created, err := repo.Create(context.Background(), newUser)

	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "alice@example.com", created.Email)
	assert.Equal(t, "fake-hash", created.PasswordHash)
}

func TestPostgresRepository_Create_GeneratesUUIDAsID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), User{
		Email:        "alice@example.com",
		PasswordHash: "fake-hash",
	})

	require.NoError(t, err)
	assert.Len(t, created.ID, 36)
}

func TestPostgresRepository_Create_GeneratesAddedAndUpdatedTimestamps(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	before := time.Now()
	created, err := repo.Create(context.Background(), User{
		Email:        "alice@example.com",
		PasswordHash: "fake-hash",
	})
	after := time.Now()

	require.NoError(t, err)
	assert.WithinRange(t, created.Added, before, after)
	assert.WithinRange(t, created.Updated, before, after)
}

func TestPostgresRepository_Create_ReturnsErrEmailAlreadyTakenOnDuplicateEmail(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.Create(context.Background(), User{
		Email:        "alice@example.com",
		PasswordHash: "fake-hash",
	})
	require.NoError(t, err)

	_, err = repo.Create(context.Background(), User{
		Email:        "alice@example.com",
		PasswordHash: "another-hash",
	})

	assert.ErrorIs(t, err, ErrEmailAlreadyTaken)
}

func TestPostgresRepository_Create_AllowsDifferentEmailsForDifferentUsers(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.Create(context.Background(), User{Email: "alice@example.com", PasswordHash: "hash1"})
	require.NoError(t, err)

	_, err = repo.Create(context.Background(), User{Email: "bob@example.com", PasswordHash: "hash2"})
	assert.NoError(t, err)
}

func TestPostgresRepository_FindByEmail_ReturnsUser(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), User{
		Email:        "alice@example.com",
		PasswordHash: "fake-hash",
	})
	require.NoError(t, err)

	result, err := repo.FindByEmail(context.Background(), "alice@example.com")

	require.NoError(t, err)
	assert.Equal(t, created.ID, result.ID)
	assert.Equal(t, "alice@example.com", result.Email)
	assert.Equal(t, "fake-hash", result.PasswordHash)
}

func TestPostgresRepository_FindByEmail_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByEmail(context.Background(), "unknown@example.com")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_FindByID_ReturnsUser(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), User{
		Email:        "alice@example.com",
		PasswordHash: "fake-hash",
	})
	require.NoError(t, err)

	result, err := repo.FindByID(context.Background(), created.ID)

	require.NoError(t, err)
	assert.Equal(t, created.ID, result.ID)
	assert.Equal(t, "alice@example.com", result.Email)
	assert.Equal(t, "fake-hash", result.PasswordHash)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), "00000000-0000-0000-0000-000000000000")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_UpdatePassword_UpdatesTheStoredHash(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), User{
		Email:        "alice@example.com",
		PasswordHash: "old-hash",
	})
	require.NoError(t, err)

	err = repo.UpdatePassword(context.Background(), created.ID, "new-hash")
	require.NoError(t, err)

	result, err := repo.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "new-hash", result.PasswordHash)
}

func TestPostgresRepository_UpdatePassword_BumpsUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), User{
		Email:        "alice@example.com",
		PasswordHash: "old-hash",
	})
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	err = repo.UpdatePassword(context.Background(), created.ID, "new-hash")
	require.NoError(t, err)

	result, err := repo.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.True(t, result.Updated.After(created.Updated))
}

func TestPostgresRepository_UpdatePassword_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	err := repo.UpdatePassword(context.Background(), "00000000-0000-0000-0000-000000000000", "new-hash")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_UpdateEmail_ChangesTheLoginEmail(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), User{Email: "alice@example.com", PasswordHash: "hash"})
	require.NoError(t, err)

	require.NoError(t, repo.UpdateEmail(context.Background(), created.ID, "alice@new.example.com"))

	result, err := repo.FindByEmail(context.Background(), "alice@new.example.com")
	require.NoError(t, err)
	assert.Equal(t, created.ID, result.ID)
	_, err = repo.FindByEmail(context.Background(), "alice@example.com")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_UpdateEmail_ReturnsErrEmailAlreadyTaken(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	alice, err := repo.Create(context.Background(), User{Email: "alice@example.com", PasswordHash: "hash"})
	require.NoError(t, err)
	_, err = repo.Create(context.Background(), User{Email: "bob@example.com", PasswordHash: "hash"})
	require.NoError(t, err)

	err = repo.UpdateEmail(context.Background(), alice.ID, "bob@example.com")

	assert.ErrorIs(t, err, ErrEmailAlreadyTaken)
}

func TestPostgresRepository_UpdateEmail_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	err := repo.UpdateEmail(context.Background(), "00000000-0000-0000-0000-000000000000", "alice@example.com")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_UpdateDisplayName_SetsAndClearsTheName(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), User{Email: "alice@example.com", PasswordHash: "hash"})
	require.NoError(t, err)
	assert.Nil(t, created.DisplayName)

	name := "Tamiyo"
	updated, err := repo.UpdateDisplayName(context.Background(), created.ID, &name)
	require.NoError(t, err)
	require.NotNil(t, updated.DisplayName)
	assert.Equal(t, "Tamiyo", *updated.DisplayName)

	found, err := repo.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, found.DisplayName)
	assert.Equal(t, "Tamiyo", *found.DisplayName)

	cleared, err := repo.UpdateDisplayName(context.Background(), created.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, cleared.DisplayName)
}

func TestPostgresRepository_UpdateDisplayName_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	name := "Tamiyo"
	_, err := repo.UpdateDisplayName(context.Background(), "00000000-0000-0000-0000-000000000000", &name)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Create_StoresTheDisplayName(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	name := "Tamiyo"
	created, err := repo.Create(context.Background(), User{Email: "alice@example.com", PasswordHash: "hash", DisplayName: &name})
	require.NoError(t, err)
	require.NotNil(t, created.DisplayName)
	assert.Equal(t, "Tamiyo", *created.DisplayName)
}

func TestPostgresRepository_UpdateAvatar_SetsAndClearsTheAvatar(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), User{Email: "alice@example.com", PasswordHash: "hash"})
	require.NoError(t, err)
	assert.Nil(t, created.AvatarScryfallID)

	id := "0000579f-7b35-4ed3-b44c-db2a538066fe"
	updated, err := repo.UpdateAvatar(context.Background(), created.ID, &id)
	require.NoError(t, err)
	require.NotNil(t, updated.AvatarScryfallID)
	assert.Equal(t, id, *updated.AvatarScryfallID)

	found, err := repo.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, found.AvatarScryfallID)
	assert.Equal(t, id, *found.AvatarScryfallID)

	cleared, err := repo.UpdateAvatar(context.Background(), created.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, cleared.AvatarScryfallID)
}

func TestPostgresRepository_UpdateAvatar_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	id := "0000579f-7b35-4ed3-b44c-db2a538066fe"
	_, err := repo.UpdateAvatar(context.Background(), "00000000-0000-0000-0000-000000000000", &id)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_RemovesTheUserAndEverythingTheyOwn(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	alice, err := repo.Create(ctx, User{Email: "alice@example.com", PasswordHash: "hash"})
	require.NoError(t, err)
	bob, err := repo.Create(ctx, User{Email: "bob@example.com", PasswordHash: "hash"})
	require.NoError(t, err)

	var storageID int
	require.NoError(t, db.Get(&storageID, `INSERT INTO tamiyo.storage (user_id, name, type) VALUES ($1, 'Binder', 'binder') RETURNING id`, alice.ID))
	var cardID int
	require.NoError(t, db.Get(&cardID, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, storage_id)
		VALUES ($1, 'Sol Ring', '0000579f-7b35-4ed3-b44c-db2a538066fe', 'cmd', '1', false, $2)
		RETURNING id
	`, alice.ID, storageID))
	var aliceDeckID string
	require.NoError(t, db.Get(&aliceDeckID, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Alice deck', 'commander') RETURNING id`, alice.ID))
	_, err = db.Exec(`INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES ($1, $2)`, cardID, aliceDeckID)
	require.NoError(t, err)
	var bobDeckID string
	require.NoError(t, db.Get(&bobDeckID, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Bob deck', 'commander') RETURNING id`, bob.ID))
	_, err = db.Exec(`INSERT INTO tamiyo.user_preferences (user_id) VALUES ($1)`, alice.ID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tamiyo.user_follows (follower_id, followed_id) VALUES ($1, $2), ($2, $1)`, alice.ID, bob.ID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tamiyo.deck_likes (user_id, deck_id) VALUES ($1, $2), ($3, $4)`, alice.ID, bobDeckID, bob.ID, aliceDeckID)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, alice.ID))

	_, err = repo.FindByID(ctx, alice.ID)
	assert.ErrorIs(t, err, ErrNotFound)

	for _, query := range []string{
		`SELECT count(*) FROM tamiyo.storage WHERE user_id = $1`,
		`SELECT count(*) FROM tamiyo.cards WHERE user_id = $1`,
		`SELECT count(*) FROM tamiyo.deck WHERE user_id = $1`,
		`SELECT count(*) FROM tamiyo.deck_pending_cards WHERE user_id = $1`,
		`SELECT count(*) FROM tamiyo.user_preferences WHERE user_id = $1`,
		`SELECT count(*) FROM tamiyo.user_follows WHERE follower_id = $1 OR followed_id = $1`,
		`SELECT count(*) FROM tamiyo.deck_likes WHERE user_id = $1`,
	} {
		var count int
		require.NoError(t, db.Get(&count, query, alice.ID))
		assert.Zero(t, count, query)
	}

	var cardDeckCount int
	require.NoError(t, db.Get(&cardDeckCount, `SELECT count(*) FROM tamiyo.card_deck WHERE deck_id = $1`, aliceDeckID))
	assert.Zero(t, cardDeckCount)
	var bobDeckLikes int
	require.NoError(t, db.Get(&bobDeckLikes, `SELECT count(*) FROM tamiyo.deck_likes WHERE deck_id = $1`, bobDeckID))
	assert.Zero(t, bobDeckLikes)

	foundBob, err := repo.FindByID(ctx, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, "bob@example.com", foundBob.Email)
	var bobDecks int
	require.NoError(t, db.Get(&bobDecks, `SELECT count(*) FROM tamiyo.deck WHERE user_id = $1`, bob.ID))
	assert.Equal(t, 1, bobDecks)
}

func TestPostgresRepository_Delete_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	err := repo.Delete(context.Background(), "00000000-0000-0000-0000-000000000000")

	assert.ErrorIs(t, err, ErrNotFound)
}
