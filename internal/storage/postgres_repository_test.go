//go:build integration

package storage

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

func setupTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	pgContainer, err := postgres.Run(ctx,
		"postgres:16",
		postgres.WithDatabase("tamiyo_test"),
		postgres.WithUsername("login"),
		postgres.WithPassword("password"),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, pgContainer.Terminate(ctx))
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	var db *sqlx.DB
	require.Eventually(t, func() bool {
		db, err = sqlx.Connect("postgres", connStr)
		return err == nil
	}, 15*time.Second, 500*time.Millisecond, "could not connect to database: %v", err)

	t.Cleanup(func() {
		db.Close()
	})

	schema, err := os.ReadFile(schemaFilePath(t))
	require.NoError(t, err)

	_, err = db.Exec(string(schema))
	require.NoError(t, err)

	return db
}

func schemaFilePath(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)

	root := filepath.Join(wd, "..", "..")
	return filepath.Join(root, "_devops", "database", "createDatabaseTables.sql")
}

func seedStorages(t *testing.T, db *sqlx.DB) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO tamiyo.storage (name, type)
		VALUES
		    ('Vintage Collection', 'binder'),
		    ('Red Deck Wins', 'deckbox');
	`)
	require.NoError(t, err)
}

func TestPostgresRepository_FindAll_ReturnsAllStorages(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	result, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestPostgresRepository_FindAll_ReturnsCorrectCardCount(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	// storage_id 1 = "Vintage Collection" (first insert in seedStorage)
	_, err := db.Exec(`
		INSERT INTO tamiyo.cards (name, scryfall_id, set_code, collector_number, foil, storage_id)
		VALUES ('Black Lotus', 'bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd', 'lea', 232, false, 1)
	`)
	require.NoError(t, err)

	result, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	require.Len(t, result, 2)

	var vintage Storage
	for _, s := range result {
		if s.Name == "Vintage Collection" {
			vintage = s
		}
	}
	assert.Equal(t, 1, vintage.CardCount)
}

func TestPostgresRepository_FindAll_ReturnsZeroCardCountForEmptyStorage(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	result, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	for _, s := range result {
		assert.Equal(t, 0, s.CardCount)
	}
}

func TestPostgresRepository_Create_InsertsAndReturnsStorageWithID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)

	newStorage := Storage{
		Name: "Vintage Collection",
		Type: "binder",
	}

	created, err := repo.Create(context.Background(), newStorage)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Vintage Collection", created.Name)

	all, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, created.ID, all[0].ID)
}

func TestPostgresRepository_Create_GeneratesAddedAndUpdatedTimestamps(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)

	before := time.Now()
	newStorage := Storage{
		Name: "Vintage Collection",
		Type: "binder",
	}

	created, err := repo.Create(context.Background(), newStorage)
	after := time.Now()

	require.NoError(t, err)
	assert.WithinRange(t, created.Added, before, after)
	assert.WithinRange(t, created.Updated, before, after)
}

func TestPostgresRepository_FindByID_ReturnsStorage(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	result, err := repo.FindByID(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, "Vintage Collection", result.Name)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_UpdatesAndReturnsStorage(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	existing, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)

	existing.Name = "Renamed Collection"
	updated, err := repo.Update(context.Background(), existing)

	require.NoError(t, err)
	assert.Equal(t, "Renamed Collection", updated.Name)
	assert.Equal(t, "binder", updated.Type)

	refetched, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "Renamed Collection", refetched.Name)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenStorageDoesNotExist(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)

	nonExistent := Storage{ID: 999, Name: "Ghost", Type: "binder"}

	_, err := repo.Update(context.Background(), nonExistent)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_RefreshesUpdatedTimestamp(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	existing, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	existing.Name = "Renamed"
	updated, err := repo.Update(context.Background(), existing)

	require.NoError(t, err)
	assert.True(t, updated.Updated.After(existing.Updated))
}
