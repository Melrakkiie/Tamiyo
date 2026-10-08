//go:build integration

package printing

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
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
		TRUNCATE TABLE tamiyo.cards, tamiyo.users, tamiyo.printings
		RESTART IDENTITY CASCADE
	`)
	require.NoError(t, err)

	return testDB
}

func seedUser(t *testing.T, db *sqlx.DB, email string) string {
	t.Helper()

	var userID string
	err := db.Get(&userID, `
		INSERT INTO tamiyo.users (email, password_hash)
		VALUES ($1, 'fake-hash')
		RETURNING id
	`, email)
	require.NoError(t, err)
	return userID
}

func seedCard(t *testing.T, db *sqlx.DB, userID, scryfallID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, mana_value)
		VALUES ($1, 'Card', $2, 'tst', '1', false, 0)
	`, userID, scryfallID)
	require.NoError(t, err)
}

func TestPostgresRepository_IDsToRefreshAndUpsert(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	const a = "11111111-1111-1111-1111-111111111111"
	const b = "22222222-2222-2222-2222-222222222222"
	const c = "33333333-3333-3333-3333-333333333333"
	seedCard(t, db, alice, a)
	seedCard(t, db, alice, a)
	seedCard(t, db, bob, a)
	seedCard(t, db, bob, b)
	seedCard(t, db, bob, c)

	ids, err := repo.IDsToRefresh(ctx, time.Now().Add(-24*time.Hour), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{a, b, c}, ids)

	require.NoError(t, repo.Upsert(ctx, []Printing{
		{ScryfallID: a, TypeLine: "Creature — Elf", LegalFormats: []string{"commander", "modern"}},
		{ScryfallID: b, TypeLine: "Instant", LegalFormats: []string{}},
	}))
	_, err = db.Exec(`UPDATE tamiyo.printings SET refreshed_at = now() - interval '2 days' WHERE scryfall_id = $1`, b)
	require.NoError(t, err)

	ids, err = repo.IDsToRefresh(ctx, time.Now().Add(-24*time.Hour), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{c, b}, ids)

	ids, err = repo.IDsToRefresh(ctx, time.Now().Add(-24*time.Hour), 1)
	require.NoError(t, err)
	assert.Equal(t, []string{c}, ids)

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Creature — Elf Druid", LegalFormats: []string{"commander"}}}))
	var typeLine string
	var formats []string
	require.NoError(t, db.QueryRow(`SELECT type_line, legal_formats FROM tamiyo.printings WHERE scryfall_id = $1`, a).Scan(&typeLine, pq.Array(&formats)))
	assert.Equal(t, "Creature — Elf Druid", typeLine)
	assert.Equal(t, []string{"commander"}, formats)
}
