//go:build integration

package deck

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

func seedDecks(t *testing.T, db *sqlx.DB) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO tamiyo.deck (name, format, commander_id)
		VALUES
		    ('Otterly Playful', 'modern', null),
		    ('Izzet Prowess', 'standard', null);
	`)
	require.NoError(t, err)
}

func TestPostgresRepository_FindAll_ReturnsAllDecks(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	result, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestPostgresRepository_FindAll_ReturnsCorrectCardCount(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	_, errCard := db.Exec(`
		INSERT INTO tamiyo.cards (name, scryfall_id, set_code, collector_number, foil, storage_id)
		VALUES ('Black Lotus', 'bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd', 'lea', 232, false, null)
	`)
	require.NoError(t, errCard)

	_, errCardDeck := db.Exec(`
		INSERT INTO tamiyo.card_deck (card_id, deck_id)
		VALUES (1, 1)
	`)
	require.NoError(t, errCardDeck)

	result, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	require.Len(t, result, 2)

	var otters Deck
	for _, d := range result {
		if d.Name == "Otterly Playful" {
			otters = d
		}
	}
	assert.Equal(t, 1, otters.CardCount)
}

func TestPostgresRepository_FindAll_ReturnsZeroCardCountForEmptyStorage(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	result, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	for _, d := range result {
		assert.Equal(t, 0, d.CardCount)
	}
}

func TestPostgresRepository_FindByID_ReturnsDeck(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	result, err := repo.FindByID(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, "Otterly Playful", result.Name)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Create_InsertsAndReturnsDeckWithID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	newDeck := Deck{
		Name:   "Otterly Playful",
		Format: "commander",
	}

	created, err := repo.Create(context.Background(), newDeck)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Otterly Playful", created.Name)

	all, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, created.ID, all[0].ID)
}

func TestPostgresRepository_Create_GeneratesAddedAndUpdatedTimestamps(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	before := time.Now()
	newDeck := Deck{
		Name:   "Otterly Playful",
		Format: "commander",
	}

	created, err := repo.Create(context.Background(), newDeck)
	after := time.Now()

	require.NoError(t, err)
	assert.WithinRange(t, created.Added, before, after)
	assert.WithinRange(t, created.Updated, before, after)
}

func TestPostgresRepository_Create_ReturnsErrCommanderNotFoundOnInvalidCommanderID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	invalidStorageID := 9999
	newDeck := Deck{
		Name:        "Otterly Playful",
		Format:      "commander",
		CommanderID: &invalidStorageID,
	}

	_, err := repo.Create(context.Background(), newDeck)

	assert.ErrorIs(t, err, ErrCommanderNotFound)
}

func TestPostgresRepository_Update_UpdatesAndReturnsDeck(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	existing, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)

	existing.Name = "Renamed Deck"
	updated, err := repo.Update(context.Background(), existing)

	require.NoError(t, err)
	assert.Equal(t, "Renamed Deck", updated.Name)
	assert.Equal(t, "modern", updated.Format)

	refetched, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "Renamed Deck", refetched.Name)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenDeckDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	nonExistent := Deck{ID: 999, Name: "Non existent", Format: "modern", CommanderID: nil}

	_, err := repo.Update(context.Background(), nonExistent)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_ReturnsErrCommanderNotFoundOnInvalidCommanderID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	existing, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)

	invalidCommanderID := 9999
	existing.Name = "Renamed Deck"
	existing.CommanderID = &invalidCommanderID
	_, errUpdate := repo.Update(context.Background(), existing)

	assert.ErrorIs(t, errUpdate, ErrCommanderNotFound)
}

func TestPostgresRepository_Update_RefreshesUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	existing, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	existing.Name = "Renamed"
	updated, err := repo.Update(context.Background(), existing)

	require.NoError(t, err)
	assert.True(t, updated.Updated.After(existing.Updated))
}

func TestPostgresRepository_Delete_RemovesDeck(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	err := repo.Delete(context.Background(), 1)

	require.NoError(t, err)

	_, err = repo.FindByID(context.Background(), 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_ReturnsErrNotFoundWhenDeckDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	err := repo.Delete(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_DoesNotAffectOtherDecks(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedDecks(t, db)

	err := repo.Delete(context.Background(), 1)
	require.NoError(t, err)

	remaining, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "Izzet Prowess", remaining[0].Name)
}
