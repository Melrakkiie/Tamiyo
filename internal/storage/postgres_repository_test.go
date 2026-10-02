//go:build integration

package storage

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
		TRUNCATE TABLE tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage, tamiyo.users
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

func seedStorages(t *testing.T, db *sqlx.DB, userID string) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO tamiyo.storage (user_id, name, type)
		VALUES
		    ($1, 'Vintage Collection', 'binder'),
		    ($1, 'Red Deck Wins', 'deckbox');
	`, userID)
	require.NoError(t, err)
}

func defaultFilter() Filter {
	return Filter{Page: 1, Limit: 25}
}

func TestPostgresRepository_FindAll_ReturnsAllStorages(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	result, total, err := repo.FindAll(context.Background(), userID, defaultFilter())

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 2, total)
}

func TestPostgresRepository_FindAll_DoesNotReturnOtherUsersStorages(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)

	result, total, err := repo.FindAll(context.Background(), userB, defaultFilter())

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Zero(t, total)
}

func TestPostgresRepository_FindAll_ReturnsCorrectCardCount(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	_, errCard := db.Exec(`
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, storage_id)
		VALUES ($1, 'Black Lotus', 'bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd', 'lea', '232', false, 1)
	`, userID)
	require.NoError(t, errCard)

	result, _, err := repo.FindAll(context.Background(), userID, defaultFilter())

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
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	result, _, err := repo.FindAll(context.Background(), userID, defaultFilter())

	require.NoError(t, err)
	for _, s := range result {
		assert.Equal(t, 0, s.CardCount)
	}
}

func TestPostgresRepository_FindAll_DoesNotCountAnotherUsersCards(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)
	seedStorages(t, db, userB) // userB's own storages: ids 3 and 4

	// userB's card lives in userB's own storage (id 3) — the
	// check_card_storage_ownership trigger forbids a card from pointing
	// at another user's storage, so this must reference a storage userB
	// actually owns for the insert to succeed.
	_, err := db.Exec(`
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, storage_id)
		VALUES ($1, 'Black Lotus', 'bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd', 'lea', '232', false, 3)
	`, userB)
	require.NoError(t, err)

	result, total, err := repo.FindAll(context.Background(), userA, defaultFilter())

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, 2, total)
	for _, s := range result {
		assert.Equal(t, 0, s.CardCount)
	}
}

func TestPostgresRepository_FindAll_FiltersByType(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID) // 'Vintage Collection'/binder, 'Red Deck Wins'/deckbox

	result, total, err := repo.FindAll(context.Background(), userID, Filter{Type: "binder", Page: 1, Limit: 25})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, "Vintage Collection", result[0].Name)
}

func TestPostgresRepository_FindAll_TypeFilterIsCaseInsensitive(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	result, _, err := repo.FindAll(context.Background(), userID, Filter{Type: "BINDER", Page: 1, Limit: 25})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Vintage Collection", result[0].Name)
}

func TestPostgresRepository_FindAll_TypeFilterReturnsEmptyWhenNoMatch(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	result, total, err := repo.FindAll(context.Background(), userID, Filter{Type: "box", Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Zero(t, total)
}

func TestPostgresRepository_FindAll_ReturnsOnlyOnePageAtATime(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID) // 2 storages total

	result, total, err := repo.FindAll(context.Background(), userID, Filter{Page: 1, Limit: 1})

	require.NoError(t, err)
	assert.Len(t, result, 1, "limit must cap the page size")
	assert.Equal(t, 2, total, "total must reflect all matching rows, not just this page")
}

func TestPostgresRepository_FindAll_ReturnsSecondPage(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID) // 2 storages total

	page1, _, err := repo.FindAll(context.Background(), userID, Filter{Page: 1, Limit: 1})
	require.NoError(t, err)
	page2, _, err := repo.FindAll(context.Background(), userID, Filter{Page: 2, Limit: 1})
	require.NoError(t, err)

	require.Len(t, page1, 1)
	require.Len(t, page2, 1)
	assert.NotEqual(t, page1[0].ID, page2[0].ID, "different pages must return different rows")
}

func TestPostgresRepository_FindAll_ReturnsEmptyPageBeyondLastPage(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID) // 2 storages total

	result, total, err := repo.FindAll(context.Background(), userID, Filter{Page: 3, Limit: 25})

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 2, total)
}

func TestPostgresRepository_FindByID_ReturnsStorage(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	result, err := repo.FindByID(context.Background(), userID, 1)

	require.NoError(t, err)
	assert.Equal(t, "Vintage Collection", result.Name)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), userID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenStorageBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)

	_, err := repo.FindByID(context.Background(), userB, 1)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Create_InsertsAndReturnsStorageWithID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	newStorage := Storage{Name: "Vintage Collection", Type: "binder"}

	created, err := repo.Create(context.Background(), userID, newStorage)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Vintage Collection", created.Name)

	all, _, err := repo.FindAll(context.Background(), userID, defaultFilter())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, created.ID, all[0].ID)
}

func TestPostgresRepository_Create_GeneratesAddedAndUpdatedTimestamps(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	before := time.Now()
	newStorage := Storage{Name: "Vintage Collection", Type: "binder"}

	created, err := repo.Create(context.Background(), userID, newStorage)
	after := time.Now()

	require.NoError(t, err)
	assert.WithinRange(t, created.Added, before, after)
	assert.WithinRange(t, created.Updated, before, after)
}

func TestPostgresRepository_Update_UpdatesAndReturnsStorage(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	existing, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)

	existing.Name = "Renamed Collection"
	updated, err := repo.Update(context.Background(), userID, existing)

	require.NoError(t, err)
	assert.Equal(t, "Renamed Collection", updated.Name)
	assert.Equal(t, "binder", updated.Type)

	refetched, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)
	assert.Equal(t, "Renamed Collection", refetched.Name)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenStorageDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	nonExistent := Storage{ID: 999, Name: "Ghost", Type: "binder"}

	_, err := repo.Update(context.Background(), userID, nonExistent)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenStorageBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)

	storageFromA := Storage{ID: 1, Name: "Hijacked", Type: "binder"}
	_, err := repo.Update(context.Background(), userB, storageFromA)

	assert.ErrorIs(t, err, ErrNotFound)

	untouched, err := repo.FindByID(context.Background(), userA, 1)
	require.NoError(t, err)
	assert.Equal(t, "Vintage Collection", untouched.Name)
}

func TestPostgresRepository_Update_RefreshesUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	existing, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	existing.Name = "Renamed"
	updated, err := repo.Update(context.Background(), userID, existing)

	require.NoError(t, err)
	assert.True(t, updated.Updated.After(existing.Updated))
}

func TestPostgresRepository_Delete_RemovesStorage(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	err := repo.Delete(context.Background(), userID, 1)

	require.NoError(t, err)

	_, err = repo.FindByID(context.Background(), userID, 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_ReturnsErrNotFoundWhenStorageDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	err := repo.Delete(context.Background(), userID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_DoesNotAffectOtherStorages(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	err := repo.Delete(context.Background(), userID, 1)
	require.NoError(t, err)

	remaining, _, err := repo.FindAll(context.Background(), userID, defaultFilter())
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "Red Deck Wins", remaining[0].Name)
}

func TestPostgresRepository_Delete_DoesNotAffectAnotherUsersStorage(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)

	err := repo.Delete(context.Background(), userB, 1)

	assert.ErrorIs(t, err, ErrNotFound)

	result, err := repo.FindByID(context.Background(), userA, 1)
	require.NoError(t, err)
	assert.Equal(t, "Vintage Collection", result.Name)
}
