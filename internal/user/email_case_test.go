//go:build integration

package user

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/_devops/database/migrations"
)

const caseInsensitiveEmailMigration = 58

func TestPostgresRepository_Create_RejectsACaseVariantOfAnExistingEmail(t *testing.T) {
	repo := NewPostgresRepository(getTestDB(t))
	_, err := repo.Create(context.Background(), User{Email: "bob@mail.fr", PasswordHash: "h"})
	require.NoError(t, err)

	_, err = repo.Create(context.Background(), User{Email: "Bob@Mail.fr", PasswordHash: "h"})

	assert.ErrorIs(t, err, ErrEmailAlreadyTaken)
}

// migrateTo runs the migrations up or down to version and puts them back at the latest on cleanup.
func migrateTo(t *testing.T, version int64) {
	t.Helper()
	goose.SetBaseFS(migrations.FS)
	t.Cleanup(func() {
		_, err := testDB.Exec(`TRUNCATE TABLE tamiyo.users CASCADE`) // a colliding fixture would fail the migration again
		require.NoError(t, err)
		require.NoError(t, goose.Up(testDB.DB, "."))
	})
	require.NoError(t, goose.DownTo(testDB.DB, ".", version))
}

func TestMigration_CaseInsensitiveEmail_LowercasesAndTrimsExistingRows(t *testing.T) {
	getTestDB(t)
	migrateTo(t, caseInsensitiveEmailMigration-1)
	_, err := testDB.Exec(`INSERT INTO tamiyo.users (email, password_hash) VALUES (' Bob@Mail.fr ', 'h'), ('alice@mail.fr', 'h')`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(testDB.DB, ".", caseInsensitiveEmailMigration))

	var emails []string
	require.NoError(t, testDB.Select(&emails, `SELECT email FROM tamiyo.users ORDER BY email`))
	assert.Equal(t, []string{"alice@mail.fr", "bob@mail.fr"}, emails)
}

func TestMigration_CaseInsensitiveEmail_FailsNamingTheCountOnCollisions(t *testing.T) {
	getTestDB(t)
	migrateTo(t, caseInsensitiveEmailMigration-1)
	_, err := testDB.Exec(`INSERT INTO tamiyo.users (email, password_hash) VALUES ('Bob@mail.fr', 'h'), ('bob@mail.fr', 'h'), ('A@x.fr', 'h'), ('a@x.fr', 'h')`)
	require.NoError(t, err)

	err = goose.UpTo(testDB.DB, ".", caseInsensitiveEmailMigration)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 address(es) collide")
	var untouched int
	require.NoError(t, testDB.Get(&untouched, `SELECT count(*) FROM tamiyo.users WHERE email <> lower(email)`))
	assert.Equal(t, 2, untouched)
}
