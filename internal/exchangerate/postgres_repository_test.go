//go:build integration

package exchangerate

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

	_, err := testDB.Exec(`TRUNCATE TABLE tamiyo.exchange_rates`)
	require.NoError(t, err)

	return testDB
}

func TestPostgresRepository_SaveAndFind(t *testing.T) {
	repo := NewPostgresRepository(getTestDB(t))
	ctx := context.Background()

	_, found, err := repo.Find(ctx, USD)
	require.NoError(t, err)
	assert.False(t, found)

	require.NoError(t, repo.Save(ctx, Rate{Currency: USD, PerEUR: 1.0876, Date: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}))
	require.NoError(t, repo.Save(ctx, Rate{Currency: USD, PerEUR: 1.0912, Date: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)}))

	rate, found, err := repo.Find(ctx, USD)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 1.0912, rate.PerEUR)
	assert.Equal(t, "2026-10-13", rate.Date.Format(time.DateOnly))
}
