//go:build integration

package card

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

func seedCards(t *testing.T, db *sqlx.DB, userID string, cards []Card) {
	t.Helper()

	for _, c := range cards {
		_, err := db.Exec(`
			INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, storage_id, mana_value)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, userID, c.Name, c.ScryfallID, c.SetCode, c.CollectorNumber, c.Foil, c.StorageID, c.ManaValue)
		require.NoError(t, err)
	}
}

func TestPostgresRepository_FindAll_ReturnsAllCardsWhenNoFilter(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	testID1, testID2 := 1, 2
	seedCards(t, db, userID, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: &testID1},
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", Foil: true, StorageID: &testID2},
	})

	result, total, err := repo.FindAll(context.Background(), userID, CardFilter{Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 2, total)
}

func TestPostgresRepository_FindAll_DoesNotReturnOtherUsersCards(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userA, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	result, total, err := repo.FindAll(context.Background(), userB, CardFilter{Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 0, total)
}

func TestPostgresRepository_FindAll_FiltersByStorageID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	testID1, testID2 := 1, 2
	seedCards(t, db, userID, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: &testID1},
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", Foil: true, StorageID: &testID2},
	})

	testID := 1
	result, total, err := repo.FindAll(context.Background(), userID, CardFilter{StorageID: &testID, Page: 1, Limit: 25})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Black Lotus", result[0].Name)
	assert.Equal(t, 1, total)
}

func TestPostgresRepository_FindAll_FiltersByNameCaseInsensitive(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userID, []Card{
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", Foil: true},
		{Name: "Lightning Helix", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "rav", CollectorNumber: "5", Foil: false},
		{Name: "Counterspell", ScryfallID: "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", SetCode: "mh2", CollectorNumber: "267", Foil: false},
	})

	result, total, err := repo.FindAll(context.Background(), userID, CardFilter{Name: "lightning", Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	names := []string{result[0].Name, result[1].Name}
	assert.Contains(t, names, "Lightning Bolt")
	assert.Contains(t, names, "Lightning Helix")
}

func TestPostgresRepository_FindAll_SortsByManaValue(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userID, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, ManaValue: 0},
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", Foil: true, ManaValue: 1},
		{Name: "Counterspell", ScryfallID: "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", SetCode: "mh2", CollectorNumber: "267", Foil: false, ManaValue: 2},
	})

	ascending, _, err := repo.FindAll(context.Background(), userID, CardFilter{SortField: "mana_value", Page: 1, Limit: 25})
	require.NoError(t, err)
	require.Len(t, ascending, 3)
	assert.Equal(t, []string{"Black Lotus", "Lightning Bolt", "Counterspell"}, []string{ascending[0].Name, ascending[1].Name, ascending[2].Name})

	descending, _, err := repo.FindAll(context.Background(), userID, CardFilter{SortField: "mana_value", SortDesc: true, Page: 1, Limit: 25})
	require.NoError(t, err)
	require.Len(t, descending, 3)
	assert.Equal(t, []string{"Counterspell", "Lightning Bolt", "Black Lotus"}, []string{descending[0].Name, descending[1].Name, descending[2].Name})
}

func TestPostgresRepository_FindAll_PaginatesResults(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	for i := 0; i < 5; i++ {
		seedCards(t, db, userID, []Card{
			{
				Name:            "Card " + string(rune('A'+i)),
				ScryfallID:      "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1" + string(rune('0'+i)),
				SetCode:         "test",
				CollectorNumber: "1",
				Foil:            false,
			},
		})
	}

	firstPage, total, err := repo.FindAll(context.Background(), userID, CardFilter{Page: 1, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, firstPage, 2)
	assert.Equal(t, 5, total)

	secondPage, total, err := repo.FindAll(context.Background(), userID, CardFilter{Page: 2, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, secondPage, 2)
	assert.Equal(t, 5, total)

	thirdPage, total, err := repo.FindAll(context.Background(), userID, CardFilter{Page: 3, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, thirdPage, 1)
	assert.Equal(t, 5, total)
}

func TestPostgresRepository_FindByID_ReturnsCard(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userID, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	result, err := repo.FindByID(context.Background(), userID, 1)

	require.NoError(t, err)
	assert.Equal(t, "Black Lotus", result.Name)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), userID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenCardBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userA, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	_, err := repo.FindByID(context.Background(), userB, 1)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Create_InsertsAndReturnsCardWithID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	testID := 1
	newCard := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: "322",
		Foil:            false,
		StorageID:       &testID,
		ManaValue:       1,
	}

	created, err := repo.Create(context.Background(), userID, newCard)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Sol Ring", created.Name)
	assert.Equal(t, 1.0, created.ManaValue)

	all, total, err := repo.FindAll(context.Background(), userID, CardFilter{StorageID: &testID, Page: 1, Limit: 25})
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, 1, total)
}

func TestPostgresRepository_Create_GeneratesAddedAndUpdatedTimestamps(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	testID := 1
	before := time.Now()
	newCard := Card{
		Name:            "Tarmogoyf",
		ScryfallID:      "3a1b2c3d-4e5f-6789-0abc-def123456789",
		SetCode:         "mm3",
		CollectorNumber: "156",
		Foil:            true,
		StorageID:       &testID,
	}

	created, err := repo.Create(context.Background(), userID, newCard)
	after := time.Now()

	require.NoError(t, err)
	assert.WithinRange(t, created.Added, before, after)
	assert.WithinRange(t, created.Updated, before, after)
}

func TestPostgresRepository_Create_ReturnsErrorOnInvalidScryfallID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	testID := 1
	invalidCard := Card{
		Name:            "Bad Card",
		ScryfallID:      "not-a-uuid",
		SetCode:         "test",
		CollectorNumber: "1",
		Foil:            false,
		StorageID:       &testID,
	}

	_, err := repo.Create(context.Background(), userID, invalidCard)

	assert.Error(t, err)
}

func TestPostgresRepository_Create_ReturnsErrStorageNotFoundOnInvalidStorageID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	invalidStorageID := 9999
	newCard := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: "322",
		Foil:            false,
		StorageID:       &invalidStorageID,
	}

	_, err := repo.Create(context.Background(), userID, newCard)

	assert.ErrorIs(t, err, ErrStorageNotFound)
}

func TestPostgresRepository_Create_ReturnsErrStorageNotFoundWhenStorageBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)

	storageID := 1
	newCard := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: "322",
		Foil:            false,
		StorageID:       &storageID,
	}

	_, err := repo.Create(context.Background(), userB, newCard)

	assert.ErrorIs(t, err, ErrStorageNotFound)
}

func TestPostgresRepository_Update_UpdatesAndReturnsCard(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userID, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	existing, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)

	existing.Name = "Renamed Card"
	updated, err := repo.Update(context.Background(), userID, existing)

	require.NoError(t, err)
	assert.Equal(t, "Renamed Card", updated.Name)

	refetched, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)
	assert.Equal(t, "Renamed Card", refetched.Name)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenCardDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	nonExistent := Card{ID: 999, Name: "Non existent", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false}

	_, err := repo.Update(context.Background(), userID, nonExistent)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenCardBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userA, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	cardFromA := Card{ID: 1, Name: "Hijacked", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false}
	_, err := repo.Update(context.Background(), userB, cardFromA)

	assert.ErrorIs(t, err, ErrNotFound)

	untouched, err := repo.FindByID(context.Background(), userA, 1)
	require.NoError(t, err)
	assert.Equal(t, "Black Lotus", untouched.Name)
}

func TestPostgresRepository_Update_ReturnsErrStorageNotFoundOnInvalidStorageID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userID, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	existing, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)

	invalidStorageID := 9999
	existing.StorageID = &invalidStorageID
	_, errUpdate := repo.Update(context.Background(), userID, existing)

	assert.ErrorIs(t, errUpdate, ErrStorageNotFound)
}

func TestPostgresRepository_Update_RefreshesUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userID, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	existing, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	existing.Name = "Renamed"
	updated, err := repo.Update(context.Background(), userID, existing)

	require.NoError(t, err)
	assert.True(t, updated.Updated.After(existing.Updated))
}

func TestPostgresRepository_Update_ReturnsErrStorageNotFoundWhenStorageBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)

	seedCards(t, db, userB, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	existing, err := repo.FindByID(context.Background(), userB, 1)
	require.NoError(t, err)

	storageID := 1
	existing.StorageID = &storageID
	_, err = repo.Update(context.Background(), userB, existing)

	assert.ErrorIs(t, err, ErrStorageNotFound)
}

func TestPostgresRepository_Delete_RemovesCard(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userID, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	err := repo.Delete(context.Background(), userID, 1)

	require.NoError(t, err)

	_, err = repo.FindByID(context.Background(), userID, 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_ReturnsErrNotFoundWhenCardDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	err := repo.Delete(context.Background(), userID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_DeleteAll_RemovesOnlyThatUsersCards(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userA, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", Foil: true},
	})
	seedCards(t, db, userB, []Card{
		{Name: "Sol Ring", ScryfallID: "6ad8011d-3471-4369-9d68-b264cc027487", SetCode: "c21", CollectorNumber: "263", Foil: false},
	})

	deleted, err := repo.DeleteAll(context.Background(), userA)

	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	_, totalA, err := repo.FindAll(context.Background(), userA, CardFilter{Page: 1, Limit: 25})
	require.NoError(t, err)
	assert.Equal(t, 0, totalA)

	_, totalB, err := repo.FindAll(context.Background(), userB, CardFilter{Page: 1, Limit: 25})
	require.NoError(t, err)
	assert.Equal(t, 1, totalB)
}

func TestPostgresRepository_Delete_DoesNotAffectAnotherUsersCard(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userA, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false},
	})

	err := repo.Delete(context.Background(), userB, 1)

	assert.ErrorIs(t, err, ErrNotFound)

	result, err := repo.FindByID(context.Background(), userA, 1)
	require.NoError(t, err)
	assert.Equal(t, "Black Lotus", result.Name)
}

func strPtr(s string) *string { return &s }

func createCardWithDetails(t *testing.T, repo *PostgresRepository, userID, name string, colors, cardType *string) Card {
	t.Helper()
	created, err := repo.Create(context.Background(), userID, Card{
		Name:            name,
		ScryfallID:      "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd",
		SetCode:         "lea",
		CollectorNumber: "1",
		Colors:          colors,
		CardType:        cardType,
	})
	require.NoError(t, err)
	return created
}

func TestPostgresRepository_Create_PersistsColorsAndType(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	created := createCardWithDetails(t, repo, userID, "Lightning Helix", strPtr("WR"), strPtr("Instant"))

	found, err := repo.FindByID(context.Background(), userID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, found.Colors)
	assert.Equal(t, "WR", *found.Colors)
	require.NotNil(t, found.CardType)
	assert.Equal(t, "Instant", *found.CardType)
}

func TestPostgresRepository_FindAll_SortsByColorGroup(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	createCardWithDetails(t, repo, userID, "Unknown Card", nil, nil)
	createCardWithDetails(t, repo, userID, "Forest", strPtr(""), strPtr("Land"))
	createCardWithDetails(t, repo, userID, "Sol Ring", strPtr(""), strPtr("Artifact"))
	createCardWithDetails(t, repo, userID, "Lightning Helix", strPtr("WR"), strPtr("Instant"))
	createCardWithDetails(t, repo, userID, "Llanowar Elves", strPtr("G"), strPtr("Creature"))
	createCardWithDetails(t, repo, userID, "Counterspell", strPtr("U"), strPtr("Instant"))
	createCardWithDetails(t, repo, userID, "Swords to Plowshares", strPtr("W"), strPtr("Instant"))

	result, _, err := repo.FindAll(context.Background(), userID, CardFilter{SortField: "color", Page: 1, Limit: 25})

	require.NoError(t, err)
	names := make([]string, len(result))
	for i, c := range result {
		names[i] = c.Name
	}
	assert.Equal(t, []string{"Swords to Plowshares", "Counterspell", "Llanowar Elves", "Lightning Helix", "Sol Ring", "Forest", "Unknown Card"}, names)
}

func TestPostgresRepository_FindAll_SortsByTypeGroupThenName(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	createCardWithDetails(t, repo, userID, "Forest", strPtr(""), strPtr("Land"))
	createCardWithDetails(t, repo, userID, "Llanowar Elves", strPtr("G"), strPtr("Creature"))
	createCardWithDetails(t, repo, userID, "Counterspell", strPtr("U"), strPtr("Instant"))
	createCardWithDetails(t, repo, userID, "Birds of Paradise", strPtr("G"), strPtr("Creature"))

	result, _, err := repo.FindAll(context.Background(), userID, CardFilter{SortField: "type", Page: 1, Limit: 25})

	require.NoError(t, err)
	names := make([]string, len(result))
	for i, c := range result {
		names[i] = c.Name
	}
	assert.Equal(t, []string{"Birds of Paradise", "Llanowar Elves", "Counterspell", "Forest"}, names)
}

func TestPostgresRepository_FindMissingDetails_AndSetDetails(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	missing := createCardWithDetails(t, repo, userA, "Sol Ring", nil, nil)
	createCardWithDetails(t, repo, userA, "Counterspell", strPtr("U"), strPtr("Instant"))
	createCardWithDetails(t, repo, userB, "Other User Card", nil, nil)

	found, err := repo.FindMissingDetails(context.Background(), userA)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, missing.ID, found[0].ID)

	require.NoError(t, repo.SetDetails(context.Background(), userA, missing.ID, Details{Colors: "", CardType: "Artifact", ManaValue: 1}))

	updated, err := repo.FindByID(context.Background(), userA, missing.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.Colors)
	assert.Equal(t, "", *updated.Colors)
	require.NotNil(t, updated.CardType)
	assert.Equal(t, "Artifact", *updated.CardType)
	assert.Equal(t, 1.0, updated.ManaValue)

	found, err = repo.FindMissingDetails(context.Background(), userA)
	require.NoError(t, err)
	assert.Empty(t, found)
}
