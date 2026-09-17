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
