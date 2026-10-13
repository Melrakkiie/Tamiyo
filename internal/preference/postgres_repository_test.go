//go:build integration

package preference

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
		TRUNCATE TABLE tamiyo.users RESTART IDENTITY CASCADE
	`)
	require.NoError(t, err)

	return testDB
}

func seedUser(t *testing.T, db *sqlx.DB, email string) string {
	t.Helper()

	var userID string
	require.NoError(t, db.Get(&userID, `INSERT INTO tamiyo.users (email, password_hash) VALUES ($1, 'fake-hash') RETURNING id`, email))
	return userID
}

func TestPostgresRepository_Preferences(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	_, found, err := repo.Find(ctx, alice)
	require.NoError(t, err)
	assert.False(t, found)

	require.NoError(t, repo.Save(ctx, alice, Defaults()))
	require.NoError(t, repo.Save(ctx, alice, Preferences{ShowCollectionInDecks: true, CardDisplay: CardDisplayText, CardColumns: 12, ShowManaCost: false, ShowCardDetails: false, CollectionGrouping: strPtr("mana"), CollectionSort: "name"}))
	require.NoError(t, repo.Save(ctx, bob, Preferences{ShowCollectionInDecks: false, CardDisplay: CardDisplayImages, CardColumns: 2, ShowManaCost: true, ShowCardDetails: true, CollectionSort: "-added"}))
	assert.Error(t, repo.Save(ctx, bob, Preferences{CardDisplay: "grid", CardColumns: 6}))

	p, found, err := repo.Find(ctx, alice)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, Preferences{ShowCollectionInDecks: true, CardDisplay: CardDisplayText, CardColumns: 12, ShowManaCost: false, ShowCardDetails: false, CollectionGrouping: strPtr("mana"), CollectionSort: "name"}, p)
	p, _, err = repo.Find(ctx, bob)
	require.NoError(t, err)
	assert.Equal(t, Preferences{ShowCollectionInDecks: false, CardDisplay: CardDisplayImages, CardColumns: 2, ShowManaCost: true, ShowCardDetails: true, CollectionSort: "-added"}, p)

	_, err = db.Exec(`DELETE FROM tamiyo.users WHERE id = $1`, bob)
	require.NoError(t, err)
	_, found, err = repo.Find(ctx, bob)
	require.NoError(t, err)
	assert.False(t, found)
}

func strPtr(s string) *string { return &s }
