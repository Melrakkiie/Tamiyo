//go:build integration

package user

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
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

	schema, err := os.ReadFile(schemaFilePath())
	if err != nil {
		panic(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		panic(err)
	}

	testDB = db

	os.Exit(m.Run())
}

func schemaFilePath() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	root := filepath.Join(wd, "..", "..")
	return filepath.Join(root, "_devops", "database", "createDatabaseTables.sql")
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
