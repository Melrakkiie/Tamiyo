//go:build integration

package follow

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

func seedUser(t *testing.T, db *sqlx.DB, email string, name string) string {
	t.Helper()

	var userID string
	require.NoError(t, db.Get(&userID, `INSERT INTO tamiyo.users (email, password_hash, display_name) VALUES ($1, 'fake-hash', $2) RETURNING id`, email, name))
	return userID
}

func TestPostgresRepository_FollowsAndConnections(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com", "Alice")
	bob := seedUser(t, db, "bob@example.com", "Bob")
	carol := seedUser(t, db, "carol@example.com", "Carol")
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	exists, err := repo.UserExists(ctx, bob)
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = repo.UserExists(ctx, "44444444-4444-4444-4444-444444444444")
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, repo.Follow(ctx, alice, bob))
	require.NoError(t, repo.Follow(ctx, alice, bob))
	require.NoError(t, repo.Follow(ctx, carol, bob))
	require.NoError(t, repo.Follow(ctx, bob, alice))
	_, err = db.Exec(`UPDATE tamiyo.user_follows SET added = now() - interval '1 day' WHERE follower_id = $1`, alice)
	require.NoError(t, err)
	assert.ErrorIs(t, repo.Follow(ctx, alice, "44444444-4444-4444-4444-444444444444"), ErrUserNotFound)
	assert.Error(t, repo.Follow(ctx, alice, alice))

	status, err := repo.Status(ctx, alice, bob)
	require.NoError(t, err)
	assert.Equal(t, Status{Followers: 2, Following: 1, FollowedByMe: true, FollowsMe: true}, status)

	followers, total, err := repo.Followers(ctx, carol, bob, Page{Number: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, followers, 2)
	assert.Equal(t, carol, followers[0].ID, "most recent first")
	assert.Equal(t, alice, followers[1].ID)
	assert.Equal(t, "Alice", *followers[1].DisplayName)
	assert.False(t, followers[1].FollowedByMe)
	assert.False(t, followers[1].Since.IsZero())

	page2, total, err := repo.Followers(ctx, carol, bob, Page{Number: 2, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, page2, 1)
	assert.Equal(t, alice, page2[0].ID)

	beyond, total, err := repo.Followers(ctx, carol, bob, Page{Number: 5, Limit: 1})
	require.NoError(t, err)
	assert.Empty(t, beyond)
	assert.Equal(t, 2, total)

	following, total, err := repo.Following(ctx, carol, carol, Page{Number: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, bob, following[0].ID)
	assert.True(t, following[0].FollowedByMe)

	require.NoError(t, repo.Unfollow(ctx, alice, bob))
	status, err = repo.Status(ctx, alice, bob)
	require.NoError(t, err)
	assert.Equal(t, Status{Followers: 1, Following: 1, FollowsMe: true}, status)

	_, err = db.Exec(`DELETE FROM tamiyo.users WHERE id = $1`, carol)
	require.NoError(t, err)
	status, err = repo.Status(ctx, alice, bob)
	require.NoError(t, err)
	assert.Equal(t, 0, status.Followers)
}
