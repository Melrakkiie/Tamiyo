//go:build integration

package card

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
		INSERT INTO tamiyo.storage (name, type, added)
		VALUES
		    ('Vintage Collection', 'binder', '2024-01-15 10:30:00'),
		    ('Red Deck Wins', 'deckbox', '2024-02-20 14:45:00');
	`)
	require.NoError(t, err)
}

func seedCards(t *testing.T, db *sqlx.DB, cards []Card) {
	t.Helper()

	for _, c := range cards {
		_, err := db.Exec(`
			INSERT INTO tamiyo.cards (name, scryfall_id, set_code, collector_number, foil, storage_id, added)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, c.Name, c.ScryfallID, c.SetCode, c.CollectorNumber, c.Foil, c.StorageID, c.Added)
		require.NoError(t, err)
	}
}

func TestPostgresRepository_FindAll_ReturnsAllCardsWhenNoFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID1, testID2 := 1, 2
	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: 232, Foil: false, StorageID: &testID1, Added: time.Now()},
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: 129, Foil: true, StorageID: &testID2, Added: time.Now()},
	})

	result, err := repo.FindAll(context.Background(), nil)

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestPostgresRepository_FindAll_FiltersByStorageID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID1, testID2 := 1, 2
	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: 232, Foil: false, StorageID: &testID1, Added: time.Now()},
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: 129, Foil: true, StorageID: &testID2, Added: time.Now()}})

	testID := 1
	result, err := repo.FindAll(context.Background(), &testID)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Black Lotus", result[0].Name)
}

func TestPostgresRepository_FindAll_ReturnsEmptySliceWhenNoStorageMatches(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID := 1
	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: 232, Foil: false, StorageID: &testID, Added: time.Now()},
	})

	unknownTestID := 67
	result, err := repo.FindAll(context.Background(), &unknownTestID)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestPostgresRepository_Create_InsertsAndReturnsCardWithID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID := 1
	newCard := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: 322,
		Foil:            false,
		StorageID:       &testID,
		Added:           time.Now().Truncate(time.Second),
	}

	created, err := repo.Create(context.Background(), newCard)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Sol Ring", created.Name)

	all, err := repo.FindAll(context.Background(), &testID)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, created.ID, all[0].ID)
}

func TestPostgresRepository_Create_InsertsAndReturnsCardWithID_NoStorageID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	newCard := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: 322,
		Foil:            false,
		Added:           time.Now().Truncate(time.Second),
	}

	created, err := repo.Create(context.Background(), newCard)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Sol Ring", created.Name)

	all, err := repo.FindAll(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, created.ID, all[0].ID)
}

func TestPostgresRepository_Create_PreservesAddedTimestamp(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	expectedAdded := time.Now().Truncate(time.Second)
	testID := 1
	newCard := Card{
		Name:            "Tarmogoyf",
		ScryfallID:      "3a1b2c3d-4e5f-6789-0abc-def123456789",
		SetCode:         "mm3",
		CollectorNumber: 156,
		Foil:            true,
		StorageID:       &testID,
		Added:           expectedAdded,
	}

	created, err := repo.Create(context.Background(), newCard)

	require.NoError(t, err)
	assert.WithinDuration(t, expectedAdded, created.Added, time.Second)
}

func TestPostgresRepository_Create_ReturnsErrorOnInvalidScryfallID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID := 1
	invalidCard := Card{
		Name:            "Bad Card",
		ScryfallID:      "not-a-uuid",
		SetCode:         "test",
		CollectorNumber: 1,
		Foil:            false,
		StorageID:       &testID,
		Added:           time.Now(),
	}

	_, err := repo.Create(context.Background(), invalidCard)

	assert.Error(t, err)
}
