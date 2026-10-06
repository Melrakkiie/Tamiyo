//go:build integration

package token

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
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"Melrakkiie/Tamiyo/_devops/database/migrations"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16",
		postgres.WithDatabase("tamiyo_test"),
		postgres.WithUsername("login"),
		postgres.WithPassword("password"),
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = pgContainer.Terminate(ctx)
	}()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	var db *sqlx.DB
	var connectErr error
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		db, connectErr = sqlx.Connect("postgres", connStr)
		if connectErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if connectErr != nil {
		panic("could not connect to database: " + connectErr.Error())
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		panic(err)
	}
	if err := goose.Up(db.DB, "."); err != nil {
		panic(err)
	}

	testDB = db

	os.Exit(m.Run())
}

func getTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	_, err := testDB.Exec(`
		TRUNCATE TABLE tamiyo.refresh_tokens, tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage, tamiyo.users
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

	created, err := repo.Create(context.Background(), RefreshToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, userID, created.UserID)
	assert.Nil(t, created.RevokedAt)
}

func TestPostgresRepository_FindByHash_ReturnsToken(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), RefreshToken{
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

func TestPostgresRepository_Revoke_SetsRevokedAt(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), RefreshToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	err = repo.Revoke(context.Background(), created.ID)
	require.NoError(t, err)

	result, err := repo.FindByHash(context.Background(), "fake-hash")
	require.NoError(t, err)
	assert.NotNil(t, result.RevokedAt)
}

func TestPostgresRepository_FindByID_ReturnsToken(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), RefreshToken{
		UserID:    userID,
		TokenHash: "fake-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	result, err := repo.FindByID(context.Background(), created.ID)

	require.NoError(t, err)
	assert.Equal(t, "fake-hash", result.TokenHash)
	assert.Nil(t, result.ReplacedBy)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), "00000000-0000-0000-0000-000000000000")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Replace_RevokesAndLinksToSuccessor(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	old, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-old", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	successor, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-new", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	err = repo.Replace(context.Background(), old.ID, successor.ID)
	require.NoError(t, err)

	result, err := repo.FindByID(context.Background(), old.ID)
	require.NoError(t, err)
	assert.NotNil(t, result.RevokedAt)
	require.NotNil(t, result.ReplacedBy)
	assert.Equal(t, successor.ID, *result.ReplacedBy)
}

func TestPostgresRepository_Replace_KeepsTheFirstSuccessor(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db)
	repo := NewPostgresRepository(db)

	old, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-old", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	first, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-first", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	second, err := repo.Create(context.Background(), RefreshToken{UserID: userID, TokenHash: "hash-second", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	require.NoError(t, repo.Replace(context.Background(), old.ID, first.ID))
	require.NoError(t, repo.Replace(context.Background(), old.ID, second.ID))

	result, err := repo.FindByID(context.Background(), old.ID)
	require.NoError(t, err)
	require.NotNil(t, result.ReplacedBy)
	assert.Equal(t, first.ID, *result.ReplacedBy)
}

func TestPostgresRepository_RevokeAllForUser_RevokesOnlyThatUsersActiveTokens(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db)

	var userB string
	err := db.Get(&userB, `
		INSERT INTO tamiyo.users (email, password_hash)
		VALUES ('bob@example.com', 'fake-hash')
		RETURNING id
	`)
	require.NoError(t, err)

	repo := NewPostgresRepository(db)

	_, err = repo.Create(context.Background(), RefreshToken{UserID: userA, TokenHash: "hash-a1", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	_, err = repo.Create(context.Background(), RefreshToken{UserID: userA, TokenHash: "hash-a2", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	_, err = repo.Create(context.Background(), RefreshToken{UserID: userB, TokenHash: "hash-b1", ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)

	err = repo.RevokeAllForUser(context.Background(), userA)
	require.NoError(t, err)

	a1, err := repo.FindByHash(context.Background(), "hash-a1")
	require.NoError(t, err)
	assert.NotNil(t, a1.RevokedAt)

	a2, err := repo.FindByHash(context.Background(), "hash-a2")
	require.NoError(t, err)
	assert.NotNil(t, a2.RevokedAt)

	b1, err := repo.FindByHash(context.Background(), "hash-b1")
	require.NoError(t, err)
	assert.Nil(t, b1.RevokedAt)
}
