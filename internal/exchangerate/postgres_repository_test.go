//go:build integration

package exchangerate

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/testdb"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m, &testDB))
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
