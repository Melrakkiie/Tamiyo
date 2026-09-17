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
	deadline := time.Now().Add(15 * time.Second)
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
		TRUNCATE TABLE tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage
		RESTART IDENTITY CASCADE
	`)
	require.NoError(t, err)

	return testDB
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
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	result, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestPostgresRepository_FindAll_ReturnsCorrectCardCount(t *testing.T) {
	db := getTestDB(t)
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
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	result, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	for _, s := range result {
		assert.Equal(t, 0, s.CardCount)
	}
}

func TestPostgresRepository_Create_InsertsAndReturnsStorageWithID(t *testing.T) {
	db := getTestDB(t)
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
	db := getTestDB(t)
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
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	result, err := repo.FindByID(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, "Vintage Collection", result.Name)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_UpdatesAndReturnsStorage(t *testing.T) {
	db := getTestDB(t)
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
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	nonExistent := Storage{ID: 999, Name: "Ghost", Type: "binder"}

	_, err := repo.Update(context.Background(), nonExistent)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_RefreshesUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
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

func TestPostgresRepository_Delete_RemovesStorage(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	err := repo.Delete(context.Background(), 1)

	require.NoError(t, err)

	_, err = repo.FindByID(context.Background(), 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_ReturnsErrNotFoundWhenStorageDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	err := repo.Delete(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_DoesNotAffectOtherStorages(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	err := repo.Delete(context.Background(), 1)
	require.NoError(t, err)

	remaining, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "Red Deck Wins", remaining[0].Name)
}

func TestPostgresRepository_Delete_SetsCardStorageIDToNull(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	_, err := db.Exec(`
		INSERT INTO tamiyo.cards (name, scryfall_id, set_code, collector_number, foil, storage_id)
		VALUES ('Black Lotus', 'bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd', 'lea', 232, false, 1)
	`)
	require.NoError(t, err)

	err = repo.Delete(context.Background(), 1)
	require.NoError(t, err)

	var storageID *int
	err = db.Get(&storageID, `SELECT storage_id FROM tamiyo.cards WHERE name = 'Black Lotus'`)
	require.NoError(t, err)
	assert.Nil(t, storageID)
}
