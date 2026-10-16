//go:build integration

package card

import (
	"context"
	"fmt"
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
		TRUNCATE TABLE tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage, tamiyo.users, tamiyo.printings, tamiyo.exchange_rates
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
			INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, userID, c.Name, c.ScryfallID, c.SetCode, c.CollectorNumber, c.Foil, c.Proxy, c.StorageID, c.ManaValue)
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

func TestPostgresRepository_FindAll_FiltersByScryfallID(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	seedCards(t, db, userA, []Card{
		{Name: "Ténèbres descendantes", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "ecl", CollectorNumber: "97"},
		{Name: "Darkness Descends", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "fdn", CollectorNumber: "12"},
	})
	seedCards(t, db, userB, []Card{
		{Name: "Darkness Descends", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "ecl", CollectorNumber: "97"},
	})

	result, total, err := repo.FindAll(context.Background(), userA, CardFilter{ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", Page: 1, Limit: 25})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Ténèbres descendantes", result[0].Name)
	assert.Equal(t, 1, total)
}

func TestPostgresRepository_FindAll_StacksCopiesOfTheSamePrintingInTheSameStorage(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)

	box := 1
	bolt := Card{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", StorageID: &box}
	foilBolt := bolt
	foilBolt.Foil = true
	looseBolt := bolt
	looseBolt.StorageID = nil
	seedCards(t, db, userID, []Card{bolt, bolt, foilBolt, looseBolt, bolt})

	result, total, err := repo.FindAll(context.Background(), userID, CardFilter{Stack: true, SortField: "name", Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, result, 3)
	quantities := map[string]int{}
	for _, c := range result {
		key := fmt.Sprintf("foil=%t storage=%v", c.Foil, c.StorageID != nil)
		quantities[key] = len(c.CopyIDs)
		assert.Equal(t, c.CopyIDs[0], c.ID)
	}
	assert.Equal(t, map[string]int{"foil=false storage=true": 3, "foil=true storage=true": 1, "foil=false storage=false": 1}, quantities)
}

func TestPostgresRepository_FindAll_PaginatesStacks(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	bolt := Card{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129"}
	counterspell := Card{Name: "Counterspell", ScryfallID: "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", SetCode: "mh2", CollectorNumber: "267"}
	seedCards(t, db, userID, []Card{bolt, bolt, counterspell, bolt})

	result, total, err := repo.FindAll(context.Background(), userID, CardFilter{Stack: true, SortField: "name", Page: 2, Limit: 1})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, result, 1)
	assert.Equal(t, "Lightning Bolt", result[0].Name)
	assert.Len(t, result[0].CopyIDs, 3)
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
		ColorIdentity:   colors,
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

func TestPostgresRepository_FindAll_SortsWithinGroups(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	forest := createCardWithDetails(t, repo, userID, "Forest", strPtr(""), strPtr("Land"))
	elves := createCardWithDetails(t, repo, userID, "Llanowar Elves", strPtr("G"), strPtr("Creature"))
	counterspell := createCardWithDetails(t, repo, userID, "Counterspell", strPtr("U"), strPtr("Instant"))
	birds := createCardWithDetails(t, repo, userID, "Birds of Paradise", strPtr("G"), strPtr("Creature"))
	thragtusk := createCardWithDetails(t, repo, userID, "Thragtusk", strPtr("G"), strPtr("Creature"))
	for id, manaValue := range map[int]float64{forest.ID: 0, elves.ID: 1, counterspell.ID: 2, birds.ID: 1, thragtusk.ID: 5} {
		_, err := db.Exec(`UPDATE tamiyo.cards SET mana_value = $1 WHERE id = $2`, manaValue, id)
		require.NoError(t, err)
	}

	names := func(filter CardFilter) []string {
		t.Helper()
		filter.Page, filter.Limit = 1, 25
		result, _, err := repo.FindAll(context.Background(), userID, filter)
		require.NoError(t, err)
		out := make([]string, len(result))
		for i, c := range result {
			out[i] = c.Name
		}
		return out
	}

	assert.Equal(t,
		[]string{"Thragtusk", "Llanowar Elves", "Birds of Paradise", "Counterspell", "Forest"},
		names(CardFilter{GroupBy: "type", SortField: "name", SortDesc: true}))
	assert.Equal(t,
		[]string{"Thragtusk", "Birds of Paradise", "Llanowar Elves", "Counterspell", "Forest"},
		names(CardFilter{GroupBy: "type", SortField: "mana_value", SortDesc: true, Stack: true}))
	assert.Equal(t,
		[]string{"Forest", "Birds of Paradise", "Llanowar Elves", "Counterspell", "Thragtusk"},
		names(CardFilter{GroupBy: "mana", SortField: "name"}))
	assert.Equal(t,
		[]string{"Counterspell", "Birds of Paradise", "Llanowar Elves", "Thragtusk", "Forest"},
		names(CardFilter{GroupBy: "color", SortField: "name"}))
}

func TestPostgresRepository_FindMissingDetails_AndSetDetails(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)

	missing := createCardWithDetails(t, repo, userA, "Sol Ring", nil, nil)
	createCardWithDetails(t, repo, userA, "Counterspell", strPtr("U"), strPtr("Instant"))
	createCardWithDetails(t, repo, userB, "Other User Card", nil, nil)

	found, err := repo.FindMissingDetails(context.Background(), userA, 0, 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, missing.ID, found[0].ID)

	count, err := repo.CountMissingDetails(context.Background(), userA, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	require.NoError(t, repo.SetDetails(context.Background(), userA, missing.ID, Details{Colors: "", CardType: "Artifact", ColorIdentity: "", ManaValue: 1}))

	updated, err := repo.FindByID(context.Background(), userA, missing.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.Colors)
	assert.Equal(t, "", *updated.Colors)
	require.NotNil(t, updated.CardType)
	assert.Equal(t, "Artifact", *updated.CardType)
	assert.Equal(t, 1.0, updated.ManaValue)

	found, err = repo.FindMissingDetails(context.Background(), userA, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestPostgresRepository_FindMissingDetails_PagesByID(t *testing.T) {
	db := getTestDB(t)
	user := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	first := createCardWithDetails(t, repo, user, "Sol Ring", nil, nil)
	second := createCardWithDetails(t, repo, user, "Arcane Signet", nil, nil)
	third := createCardWithDetails(t, repo, user, "Mind Stone", nil, nil)

	page, err := repo.FindMissingDetails(context.Background(), user, 0, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, first.ID, page[0].ID)
	assert.Equal(t, second.ID, page[1].ID)

	remaining, err := repo.CountMissingDetails(context.Background(), user, second.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, remaining)

	page, err = repo.FindMissingDetails(context.Background(), user, second.ID, 2)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, third.ID, page[0].ID)
}

func TestPostgresRepository_FindAll_FiltersByColorIdentity(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	createCardWithDetails(t, repo, userID, "Lightning Helix", strPtr("WR"), strPtr("Instant"))
	createCardWithDetails(t, repo, userID, "Swords to Plowshares", strPtr("W"), strPtr("Instant"))
	createCardWithDetails(t, repo, userID, "Sol Ring", strPtr(""), strPtr("Artifact"))
	createCardWithDetails(t, repo, userID, "Counterspell", strPtr("U"), strPtr("Instant"))
	createCardWithDetails(t, repo, userID, "Unknown Card", nil, nil)

	names := func(identity string) []string {
		cards, total, err := repo.FindAll(context.Background(), userID, CardFilter{
			ColorIdentity: &identity, SortField: "name", Page: 1, Limit: 25,
		})
		require.NoError(t, err)
		result := make([]string, len(cards))
		for i, c := range cards {
			result[i] = c.Name
		}
		assert.Len(t, result, total)
		return result
	}

	assert.Equal(t, []string{"Lightning Helix", "Sol Ring", "Swords to Plowshares"}, names("WR"))
	assert.Equal(t, []string{"Sol Ring", "Swords to Plowshares"}, names("W"))
	assert.Equal(t, []string{"Sol Ring"}, names(""))
	assert.Equal(t, []string{"Counterspell", "Lightning Helix", "Sol Ring", "Swords to Plowshares"}, names("WUBRG"))
}

func TestPostgresRepository_Delete_KeepsDeckCardsAsPending(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	first := createCardWithDetails(t, repo, userID, "Sol Ring", strPtr(""), strPtr("Artifact"))
	second := createCardWithDetails(t, repo, userID, "Sol Ring", strPtr(""), strPtr("Artifact"))
	var deckID string
	require.NoError(t, db.Get(&deckID, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Deck', 'commander') RETURNING id`, userID))
	_, err := db.Exec(`INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES ($1, $3), ($2, $3)`, first.ID, second.ID, deckID)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(context.Background(), userID, first.ID))

	type pendingRow struct {
		Name     string `db:"name"`
		Quantity int    `db:"quantity"`
	}
	var pending []pendingRow
	require.NoError(t, db.Select(&pending, `SELECT name, quantity FROM tamiyo.deck_pending_cards WHERE deck_id = $1`, deckID))
	require.Len(t, pending, 1)
	assert.Equal(t, pendingRow{Name: "Sol Ring", Quantity: 1}, pending[0])

	deleted, err := repo.DeleteAll(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	pending = nil
	require.NoError(t, db.Select(&pending, `SELECT name, quantity FROM tamiyo.deck_pending_cards WHERE deck_id = $1`, deckID))
	require.Len(t, pending, 1)
	assert.Equal(t, 2, pending[0].Quantity)
}

func TestPostgresRepository_CreateAndUpdate_PersistProxy(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), userID, Card{Name: "Mana Crypt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "270", Proxy: true})
	require.NoError(t, err)
	assert.True(t, created.Proxy)

	created.Proxy = false
	updated, err := repo.Update(context.Background(), userID, created)
	require.NoError(t, err)
	assert.False(t, updated.Proxy)

	found, err := repo.FindByID(context.Background(), userID, created.ID)
	require.NoError(t, err)
	assert.False(t, found.Proxy)
}

func TestPostgresRepository_FindAll_DoesNotStackProxiesWithRealCopies(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	crypt := Card{Name: "Mana Crypt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "270"}
	proxy := crypt
	proxy.Proxy = true
	for _, c := range []Card{crypt, proxy, proxy} {
		_, err := repo.Create(context.Background(), userID, c)
		require.NoError(t, err)
	}

	result, total, err := repo.FindAll(context.Background(), userID, CardFilter{Stack: true, SortField: "name", Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	quantities := map[bool]int{}
	for _, c := range result {
		quantities[c.Proxy] = len(c.CopyIDs)
	}
	assert.Equal(t, map[bool]int{false: 1, true: 2}, quantities)
}

func TestPostgresRepository_FindAll_GroupsBySubtype(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	const wizard = "11111111-1111-1111-1111-111111111111"
	const flip = "22222222-2222-2222-2222-222222222222"
	const bolt = "33333333-3333-3333-3333-333333333333"
	_, err := db.Exec(`
		INSERT INTO tamiyo.printings (scryfall_id, type_line) VALUES
			($1, 'Creature — Human Wizard'),
			($2, 'Legendary Creature — Human Werewolf // Legendary Planeswalker — Tibalt'),
			($3, 'Instant')
	`, wizard, flip, bolt)
	require.NoError(t, err)
	seedCards(t, db, userID, []Card{
		{Name: "Snapcaster Mage", ScryfallID: wizard, SetCode: "tst", CollectorNumber: "1"},
		{Name: "Snapcaster Mage", ScryfallID: wizard, SetCode: "tst", CollectorNumber: "1"},
		{Name: "Tibalt, Cosmic Impostor", ScryfallID: flip, SetCode: "tst", CollectorNumber: "2"},
		{Name: "Lightning Bolt", ScryfallID: bolt, SetCode: "tst", CollectorNumber: "3"},
		{Name: "Unknown", ScryfallID: "44444444-4444-4444-4444-444444444444", SetCode: "tst", CollectorNumber: "4"},
	})

	groups := func(filter CardFilter) ([]string, int) {
		t.Helper()
		filter.GroupBy, filter.SortField = "subtype", "name"
		if filter.Limit == 0 {
			filter.Page, filter.Limit = 1, 25
		}
		cards, total, err := repo.FindAll(context.Background(), userID, filter)
		require.NoError(t, err)
		out := make([]string, 0, len(cards))
		for _, c := range cards {
			subtype := "-"
			if c.Subtype != nil {
				subtype = *c.Subtype
			}
			out = append(out, subtype+": "+c.Name)
		}
		return out, total
	}

	rows, total := groups(CardFilter{})
	assert.Equal(t, 9, total)
	assert.Equal(t, []string{
		"Human: Snapcaster Mage", "Human: Snapcaster Mage", "Human: Tibalt, Cosmic Impostor",
		"Tibalt: Tibalt, Cosmic Impostor", "Werewolf: Tibalt, Cosmic Impostor",
		"Wizard: Snapcaster Mage", "Wizard: Snapcaster Mage",
		"-: Lightning Bolt", "-: Unknown",
	}, rows)

	rows, total = groups(CardFilter{Stack: true})
	assert.Equal(t, 7, total)
	assert.Equal(t, []string{
		"Human: Snapcaster Mage", "Human: Tibalt, Cosmic Impostor",
		"Tibalt: Tibalt, Cosmic Impostor", "Werewolf: Tibalt, Cosmic Impostor",
		"Wizard: Snapcaster Mage",
		"-: Lightning Bolt", "-: Unknown",
	}, rows)

	rows, total = groups(CardFilter{Stack: true, Page: 2, Limit: 2})
	assert.Equal(t, 7, total)
	assert.Equal(t, []string{"Tibalt: Tibalt, Cosmic Impostor", "Werewolf: Tibalt, Cosmic Impostor"}, rows)

	cards, _, err := repo.FindAll(context.Background(), userID, CardFilter{SortField: "name", Page: 1, Limit: 25})
	require.NoError(t, err)
	assert.Len(t, cards, 5)
	assert.Nil(t, cards[0].Subtype)
}

func TestPostgresRepository_FindAll_AdvancedFilters(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	var binderID, deckboxID int
	require.NoError(t, db.Get(&binderID, `INSERT INTO tamiyo.storage (user_id, name, type) VALUES ($1, 'Binder', 'binder') RETURNING id`, userID))
	require.NoError(t, db.Get(&deckboxID, `INSERT INTO tamiyo.storage (user_id, name, type) VALUES ($1, 'Kess', 'Deckbox ') RETURNING id`, userID))

	type seed struct {
		name, scryfallID, colors, cardType, typeLine string
		manaValue                                    float64
		foil                                         bool
		storageID                                    *int
		legal                                        []string
	}
	seeds := []seed{
		{"Llanowar Elves", "11111111-1111-1111-1111-111111111111", "G", "Creature", "Creature — Elf Druid", 1, false, &binderID, []string{"modern", "commander"}},
		{"Lightning Helix", "22222222-2222-2222-2222-222222222222", "RW", "Instant", "Instant", 2, true, &deckboxID, []string{"modern", "commander"}},
		{"Sol Ring", "33333333-3333-3333-3333-333333333333", "", "Artifact", "Artifact", 1, false, &deckboxID, []string{"commander"}},
		{"Esika's Chariot", "44444444-4444-4444-4444-444444444444", "G", "Artifact", "Legendary Artifact — Vehicle", 4, true, nil, []string{"commander", "pioneer"}},
		{"Grist, the Hunger Tide", "55555555-5555-5555-5555-555555555555", "BG", "Planeswalker", "Legendary Planeswalker — Grist", 3, false, &binderID, []string{"modern"}},
		{"Unknown", "66666666-6666-6666-6666-666666666666", "", "", "", 7, false, nil, nil},
	}
	for _, s := range seeds {
		c := Card{Name: s.name, ScryfallID: s.scryfallID, SetCode: "tst", CollectorNumber: "1", Foil: s.foil, StorageID: s.storageID, ManaValue: s.manaValue}
		if s.name != "Unknown" {
			colors, cardType := s.colors, s.cardType
			c.Colors, c.CardType, c.ColorIdentity = &colors, &cardType, &colors
		}
		_, err := repo.Create(ctx, userID, c)
		require.NoError(t, err)
		if s.typeLine != "" {
			_, err = db.Exec(`INSERT INTO tamiyo.printings (scryfall_id, type_line, legal_formats) VALUES ($1, $2, $3)`, s.scryfallID, s.typeLine, pq.Array(s.legal))
			require.NoError(t, err)
		}
	}

	names := func(filter CardFilter) []string {
		t.Helper()
		filter.Page, filter.Limit, filter.SortField = 1, 25, "name"
		result, _, err := repo.FindAll(ctx, userID, filter)
		require.NoError(t, err)
		out := make([]string, len(result))
		for i, c := range result {
			out[i] = c.Name
		}
		return out
	}
	str := func(s string) *string { return &s }
	num := func(f float64) *float64 { return &f }
	count := func(i int) *int { return &i }
	yes := true

	assert.Equal(t, []string{"Esika's Chariot", "Llanowar Elves"}, names(CardFilter{Colors: str("G"), ColorMode: ColorModeExact}))
	assert.Equal(t, []string{"Esika's Chariot", "Grist, the Hunger Tide", "Llanowar Elves"}, names(CardFilter{Colors: str("G"), ColorMode: ColorModeInclude}))
	assert.Equal(t, []string{"Esika's Chariot", "Llanowar Elves", "Sol Ring"}, names(CardFilter{Colors: str("G"), ColorMode: ColorModeWithin}))
	assert.Equal(t, []string{"Sol Ring"}, names(CardFilter{Colors: str(""), ColorMode: ColorModeExact}))
	assert.Equal(t, []string{"Esika's Chariot", "Grist, the Hunger Tide", "Unknown"}, names(CardFilter{ManaValue: num(3), ManaValueOp: "gte"}))
	assert.Equal(t, []string{"Llanowar Elves", "Sol Ring"}, names(CardFilter{ManaValue: num(1), ManaValueOp: "eq"}))
	assert.Equal(t, []string{"Esika's Chariot", "Sol Ring"}, names(CardFilter{Type: "Artifact"}))
	assert.Equal(t, []string{"Llanowar Elves"}, names(CardFilter{Subtype: "elf"}))
	assert.Equal(t, []string{"Esika's Chariot"}, names(CardFilter{Subtype: "vehicle", Type: "Artifact"}))
	assert.Equal(t, []string{"Esika's Chariot"}, names(CardFilter{LegalIn: "pioneer"}))
	assert.Equal(t, []string{"Grist, the Hunger Tide", "Lightning Helix"}, names(CardFilter{ColorCount: count(2)}))
	assert.Equal(t, []string{"Esika's Chariot", "Lightning Helix"}, names(CardFilter{Foil: &yes}))
	assert.Equal(t, []string{"Lightning Helix", "Sol Ring"}, names(CardFilter{StorageType: "deckbox"}))
	assert.Equal(t, []string{"Lightning Helix"}, names(CardFilter{StorageType: "deckbox", LegalIn: "modern", Stack: true}))

	var allowedDeck, otherDeck string
	require.NoError(t, db.Get(&allowedDeck, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Allowed', 'commander') RETURNING id`, userID))
	require.NoError(t, db.Get(&otherDeck, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Other', 'commander') RETURNING id`, userID))
	_, err := db.Exec(`UPDATE tamiyo.storage SET locked = true WHERE id = $1`, deckboxID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tamiyo.storage_allowed_deck (storage_id, deck_id) VALUES ($1, $2)`, deckboxID, allowedDeck)
	require.NoError(t, err)

	everything := []string{"Esika's Chariot", "Grist, the Hunger Tide", "Lightning Helix", "Llanowar Elves", "Sol Ring", "Unknown"}
	assert.Equal(t, everything, names(CardFilter{OpenToDeck: str(allowedDeck)}))
	assert.Equal(t, []string{"Esika's Chariot", "Grist, the Hunger Tide", "Llanowar Elves", "Unknown"}, names(CardFilter{OpenToDeck: str(otherDeck)}))
	assert.Equal(t, []string{"Esika's Chariot", "Llanowar Elves"}, names(CardFilter{OpenToDeck: str(otherDeck), Colors: str("G"), ColorMode: ColorModeExact, Stack: true}))
	assert.Equal(t, everything, names(CardFilter{}))
}

func TestPostgresRepository_FindAll_FiltersByPowerAndToughness(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	seeds := []struct {
		name, scryfallID    string
		powers, toughnesses []string
	}{
		{"Llanowar Elves", "11111111-1111-1111-1111-111111111111", []string{"1"}, []string{"1"}},
		{"Delver of Secrets", "22222222-2222-2222-2222-222222222222", []string{"1", "3"}, []string{"1", "2"}},
		{"Tarmogoyf", "33333333-3333-3333-3333-333333333333", []string{"*"}, []string{"1+*"}},
		{"Esika's Chariot", "44444444-4444-4444-4444-444444444444", []string{"4"}, []string{"4"}},
		{"Sol Ring", "55555555-5555-5555-5555-555555555555", []string{}, []string{}},
		{"Half Squirrel", "66666666-6666-6666-6666-666666666666", []string{".5"}, []string{"-1"}},
	}
	for _, s := range seeds {
		_, err := repo.Create(ctx, userID, Card{Name: s.name, ScryfallID: s.scryfallID, SetCode: "tst", CollectorNumber: "1"})
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO tamiyo.printings (scryfall_id, type_line, powers, toughnesses) VALUES ($1, 'Creature', $2, $3)`,
			s.scryfallID, pq.Array(s.powers), pq.Array(s.toughnesses))
		require.NoError(t, err)
	}
	_, err := repo.Create(ctx, userID, Card{Name: "Unknown", ScryfallID: "77777777-7777-7777-7777-777777777777", SetCode: "tst", CollectorNumber: "1"})
	require.NoError(t, err)

	names := func(filter CardFilter) []string {
		t.Helper()
		filter.Page, filter.Limit, filter.SortField = 1, 25, "name"
		result, _, err := repo.FindAll(ctx, userID, filter)
		require.NoError(t, err)
		out := make([]string, len(result))
		for i, c := range result {
			out[i] = c.Name
		}
		return out
	}
	num := func(f float64) *float64 { return &f }

	assert.Equal(t, []string{"Delver of Secrets", "Llanowar Elves"}, names(CardFilter{Power: num(1), PowerOp: "eq"}))
	assert.Equal(t, []string{"Delver of Secrets", "Esika's Chariot"}, names(CardFilter{Power: num(3), PowerOp: "gte"}))
	assert.Equal(t, []string{"Half Squirrel"}, names(CardFilter{Power: num(1), PowerOp: "lt"}))
	assert.Empty(t, names(CardFilter{Power: num(0), PowerOp: "eq"}))
	assert.Equal(t, []string{"Half Squirrel"}, names(CardFilter{Toughness: num(0), ToughnessOp: "lt"}))
	assert.Equal(t, []string{"Delver of Secrets"}, names(CardFilter{Toughness: num(2), ToughnessOp: "eq"}))
	assert.Equal(t, []string{"Delver of Secrets"}, names(CardFilter{Power: num(3), PowerOp: "eq", Toughness: num(1), ToughnessOp: "eq", Stack: true}))
}

func TestPostgresRepository_FindAll_SearchQuery(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	type seed struct {
		name, scryfallID, colors, cardType, set, number string
		manaValue                                      float64
		foil                                           bool
		typeLine, oracle, manaCost, rarity             string
		legal, keywords, powers                        []string
		usd                                            float64
	}
	seeds := []seed{
		{"Llanowar Elves", "11111111-1111-1111-1111-111111111111", "G", "Creature", "dom", "168", 1, false,
			"Creature — Elf Druid", "{T}: Add {G}.", "{G}", "common", []string{"modern", "commander"}, []string{}, []string{"1"}, 0.5},
		{"Lightning Helix", "22222222-2222-2222-2222-222222222222", "WR", "Instant", "rav", "213", 2, true,
			"Instant", "Lightning Helix deals 3 damage to any target and you gain 3 life.", "{R}{W}", "uncommon", []string{"modern", "commander"}, []string{}, []string{}, 4},
		{"Sol Ring", "33333333-3333-3333-3333-333333333333", "", "Artifact", "c21", "263", 1, false,
			"Artifact", "{T}: Add {C}{C}.", "{1}", "uncommon", []string{"commander"}, []string{}, []string{}, 2},
		{"Questing Beast", "44444444-4444-4444-4444-444444444444", "G", "Creature", "eld", "171", 4, false,
			"Legendary Creature — Beast", "Vigilance, deathtouch, haste\nQuesting Beast can't be blocked by creatures with power 2 or less.", "{2}{G}{G}", "mythic",
			[]string{"modern", "commander"}, []string{"Vigilance", "Deathtouch", "Haste"}, []string{"4"}, 10},
		{"Forest", "55555555-5555-5555-5555-555555555555", "", "Land", "dom", "280", 0, false,
			"Basic Land — Forest", "({T}: Add {G}.)", "", "common", []string{"modern", "commander"}, []string{}, []string{}, 0.1},
		{"Mystery Elf", "66666666-6666-6666-6666-666666666666", "U", "Creature", "tst", "1", 7, false,
			"", "", "", "", nil, nil, nil, 0},
	}
	for _, s := range seeds {
		colors, cardType := s.colors, s.cardType
		_, err := repo.Create(ctx, userID, Card{
			Name: s.name, ScryfallID: s.scryfallID, SetCode: s.set, CollectorNumber: s.number, Foil: s.foil, ManaValue: s.manaValue,
			Colors: &colors, CardType: &cardType, ColorIdentity: &colors,
		})
		require.NoError(t, err)
		if s.typeLine == "" {
			continue
		}
		var usd, usdFoil *float64
		if s.foil {
			usdFoil = &s.usd
		} else {
			usd = &s.usd
		}
		_, err = db.Exec(`
			INSERT INTO tamiyo.printings (scryfall_id, type_line, oracle_text, mana_cost, rarity, legal_formats, keywords, powers, usd, usd_foil)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10)
		`, s.scryfallID, s.typeLine, s.oracle, s.manaCost, s.rarity, pq.Array(s.legal), pq.Array(s.keywords), pq.Array(s.powers), usd, usdFoil)
		require.NoError(t, err)
	}

	names := func(query string, stack bool) []string {
		t.Helper()
		node, err := parseSearchQuery(query)
		require.NoError(t, err)
		result, _, err := repo.FindAll(ctx, userID, CardFilter{Query: node, Stack: stack, Page: 1, Limit: 25, SortField: "name"})
		require.NoError(t, err)
		out := make([]string, len(result))
		for i, c := range result {
			out[i] = c.Name
		}
		return out
	}

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"t:creature c:g mv<=3", []string{"Llanowar Elves"}},
		{"t:creature", []string{"Llanowar Elves", "Mystery Elf", "Questing Beast"}},
		{"t:elf", []string{"Llanowar Elves"}},
		{`o:"add {g}"`, []string{"Forest", "Llanowar Elves"}},
		{`o:"~ deals"`, []string{"Lightning Helix"}},
		{`o:"~ can't be blocked"`, []string{"Questing Beast"}},
		{"c:g", []string{"Llanowar Elves", "Questing Beast"}},
		{"c=rw", []string{"Lightning Helix"}},
		{"c<=boros", []string{"Forest", "Lightning Helix", "Sol Ring"}},
		{"c:c", []string{"Forest", "Sol Ring"}},
		{"c:m", []string{"Lightning Helix"}},
		{"c>=1", []string{"Lightning Helix", "Llanowar Elves", "Mystery Elf", "Questing Beast"}},
		{"id:g", []string{"Forest", "Llanowar Elves", "Questing Beast", "Sol Ring"}},
		{"m:G", []string{"Llanowar Elves", "Questing Beast"}},
		{"m:GG", []string{"Questing Beast"}},
		{"m=G", []string{"Llanowar Elves"}},
		{"m>G", []string{"Questing Beast"}},
		{"r:mythic", []string{"Questing Beast"}},
		{"r>=uncommon", []string{"Lightning Helix", "Questing Beast", "Sol Ring"}},
		{"r<rare", []string{"Forest", "Lightning Helix", "Llanowar Elves", "Sol Ring"}},
		{"-f:modern", []string{"Mystery Elf", "Sol Ring"}},
		{"kw:deathtouch", []string{"Questing Beast"}},
		{"pow>=4", []string{"Questing Beast"}},
		{"usd>=2", []string{"Lightning Helix", "Questing Beast", "Sol Ring"}},
		{"is:foil", []string{"Lightning Helix"}},
		{"not:foil mv>3", []string{"Mystery Elf", "Questing Beast"}},
		{"t:creature or is:foil", []string{"Lightning Helix", "Llanowar Elves", "Mystery Elf", "Questing Beast"}},
		{"-(t:creature or t:land) mv<2", []string{"Sol Ring"}},
		{"s:DOM cn:168", []string{"Llanowar Elves"}},
		{"cn>200", []string{"Forest", "Lightning Helix", "Sol Ring"}},
		{"mv:even", []string{"Forest", "Lightning Helix", "Questing Beast"}},
		{`!"sol ring"`, []string{"Sol Ring"}},
		{"elf", []string{"Mystery Elf"}},
		{"100%", []string{}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			assert.Equal(t, tc.want, names(tc.query, false))
		})
	}

	assert.Equal(t, []string{"Llanowar Elves", "Questing Beast"}, names("t:creature c:g", true))
}

func TestPostgresRepository_Value_SumsPricesOfOwnedNonProxyCards(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	const both = "11111111-1111-1111-1111-111111111111"
	const foilOnly = "22222222-2222-2222-2222-222222222222"
	const unpriced = "33333333-3333-3333-3333-333333333333"
	const unknown = "44444444-4444-4444-4444-444444444444"
	const dollarsOnly = "55555555-5555-5555-5555-555555555555"
	const etchedOnly = "66666666-6666-6666-6666-666666666666"
	_, err := db.Exec(`
		INSERT INTO tamiyo.printings (scryfall_id, type_line, eur, eur_foil, usd, usd_foil, usd_etched) VALUES
			($1, 'Instant', 1.00, 5.00, 1.50, 6.00, NULL),
			($2, 'Instant', NULL, 3.25, NULL, 4.00, NULL),
			($3, 'Instant', NULL, NULL, NULL, NULL, NULL),
			($4, 'Instant', NULL, NULL, 2.50, NULL, NULL),
			($5, 'Instant', NULL, NULL, NULL, NULL, 10.00)
	`, both, foilOnly, unpriced, dollarsOnly, etchedOnly)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tamiyo.exchange_rates (currency, per_eur, rate_date) VALUES ('usd', 1.25, '2026-10-13')`)
	require.NoError(t, err)
	seedCards(t, db, alice, []Card{
		{Name: "A", ScryfallID: both, SetCode: "tst", CollectorNumber: "1"},
		{Name: "A", ScryfallID: both, SetCode: "tst", CollectorNumber: "1", Foil: true},
		{Name: "A", ScryfallID: both, SetCode: "tst", CollectorNumber: "1", Proxy: true},
		{Name: "B", ScryfallID: foilOnly, SetCode: "tst", CollectorNumber: "2"},
		{Name: "C", ScryfallID: unpriced, SetCode: "tst", CollectorNumber: "3"},
		{Name: "D", ScryfallID: unknown, SetCode: "tst", CollectorNumber: "4"},
		{Name: "E", ScryfallID: dollarsOnly, SetCode: "tst", CollectorNumber: "5"},
		{Name: "F", ScryfallID: etchedOnly, SetCode: "tst", CollectorNumber: "6", Foil: true},
	})
	seedCards(t, db, bob, []Card{{Name: "A", ScryfallID: both, SetCode: "tst", CollectorNumber: "1", Foil: true}})

	value, err := repo.Value(context.Background(), alice)
	require.NoError(t, err)
	assert.InDelta(t, 19.25, value.EUR, 0.001)
	assert.InDelta(t, 24, value.USD, 0.001)

	empty, err := repo.Value(context.Background(), seedUser(t, db, "carol@example.com"))
	require.NoError(t, err)
	assert.Equal(t, Value{}, empty)
}

func TestPostgresRepository_FindAll_SortsByPriceInThePreferredCurrency(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	const cheapEuros = "11111111-1111-1111-1111-111111111111"
	const dollarsOnly = "22222222-2222-2222-2222-222222222222"
	const unpriced = "33333333-3333-3333-3333-333333333333"
	_, err := db.Exec(`
		INSERT INTO tamiyo.printings (scryfall_id, type_line, eur, eur_foil, usd, usd_foil, usd_etched) VALUES
			($1, 'Instant', 1.00, 8.00, 9.00, 20.00, NULL),
			($2, 'Instant', NULL, NULL, 5.00, NULL, NULL),
			($3, 'Instant', NULL, NULL, NULL, NULL, NULL)
	`, cheapEuros, dollarsOnly, unpriced)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tamiyo.exchange_rates (currency, per_eur, rate_date) VALUES ('usd', 1.25, '2026-10-13')`)
	require.NoError(t, err)
	seedCards(t, db, alice, []Card{
		{Name: "Cheap", ScryfallID: cheapEuros, SetCode: "tst", CollectorNumber: "1"},
		{Name: "Cheap Foil", ScryfallID: cheapEuros, SetCode: "tst", CollectorNumber: "1", Foil: true},
		{Name: "Dollars", ScryfallID: dollarsOnly, SetCode: "tst", CollectorNumber: "2"},
		{Name: "Unpriced", ScryfallID: unpriced, SetCode: "tst", CollectorNumber: "3"},
		{Name: "Unknown", ScryfallID: "44444444-4444-4444-4444-444444444444", SetCode: "tst", CollectorNumber: "4"},
	})

	names := func(filter CardFilter) []string {
		filter.Page, filter.Limit, filter.SortField = 1, 25, "price"
		cards, _, err := repo.FindAll(context.Background(), alice, filter)
		require.NoError(t, err)
		out := make([]string, 0, len(cards))
		for _, c := range cards {
			out = append(out, c.Name)
		}
		return out
	}

	assert.Equal(t, []string{"Cheap", "Dollars", "Cheap Foil", "Unknown", "Unpriced"}, names(CardFilter{}))
	assert.Equal(t, []string{"Cheap Foil", "Dollars", "Cheap", "Unknown", "Unpriced"}, names(CardFilter{SortDesc: true}))
	assert.Equal(t, []string{"Cheap Foil", "Dollars", "Cheap", "Unknown", "Unpriced"}, names(CardFilter{SortDesc: true, Stack: true}))

	_, err = db.Exec(`INSERT INTO tamiyo.user_preferences (user_id, price_currency) VALUES ($1, 'usd')`, alice)
	require.NoError(t, err)
	assert.Equal(t, []string{"Dollars", "Cheap", "Cheap Foil", "Unknown", "Unpriced"}, names(CardFilter{}))
	assert.Equal(t, []string{"Cheap Foil", "Cheap", "Dollars", "Unknown", "Unpriced"}, names(CardFilter{SortDesc: true, Stack: true}))
}

func TestPostgresRepository_MoveToStorage(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)
	ctx := context.Background()

	binder, deckbox := 1, 2
	loose := createCardWithDetails(t, repo, userA, "Sol Ring", nil, nil)
	inBinder, err := repo.Create(ctx, userA, Card{Name: "Counterspell", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "1", StorageID: &binder})
	require.NoError(t, err)
	already, err := repo.Create(ctx, userA, Card{Name: "Island", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "1", StorageID: &deckbox})
	require.NoError(t, err)
	unlisted := createCardWithDetails(t, repo, userA, "Forest", nil, nil)
	foreign := createCardWithDetails(t, repo, userB, "Other User Card", nil, nil)

	moved, err := repo.MoveToStorage(ctx, userA, []int{loose.ID, inBinder.ID, already.ID, foreign.ID}, deckbox)

	require.NoError(t, err)
	assert.Equal(t, 2, moved)
	for _, id := range []int{loose.ID, inBinder.ID, already.ID} {
		c, err := repo.FindByID(ctx, userA, id)
		require.NoError(t, err)
		require.NotNil(t, c.StorageID)
		assert.Equal(t, deckbox, *c.StorageID)
	}
	c, err := repo.FindByID(ctx, userA, unlisted.ID)
	require.NoError(t, err)
	assert.Nil(t, c.StorageID)
	c, err = repo.FindByID(ctx, userB, foreign.ID)
	require.NoError(t, err)
	assert.Nil(t, c.StorageID)
}

func TestPostgresRepository_MoveToStorage_ReturnsErrStorageNotFound(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)
	foreign := createCardWithDetails(t, repo, userB, "Other User Card", nil, nil)

	_, err := repo.MoveToStorage(context.Background(), userB, []int{foreign.ID}, 1)

	assert.ErrorIs(t, err, ErrStorageNotFound)
}

func TestPostgresRepository_MoveStorageCards(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)
	ctx := context.Background()

	binder, deckbox := 1, 2
	scryfallID := "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd"
	first, err := repo.Create(ctx, userA, Card{Name: "Counterspell", ScryfallID: scryfallID, SetCode: "lea", CollectorNumber: "1", StorageID: &binder})
	require.NoError(t, err)
	second, err := repo.Create(ctx, userA, Card{Name: "Island", ScryfallID: scryfallID, SetCode: "lea", CollectorNumber: "1", StorageID: &binder})
	require.NoError(t, err)
	stays, err := repo.Create(ctx, userA, Card{Name: "Swamp", ScryfallID: scryfallID, SetCode: "lea", CollectorNumber: "1", StorageID: &deckbox})
	require.NoError(t, err)
	loose := createCardWithDetails(t, repo, userA, "Sol Ring", nil, nil)
	foreign := createCardWithDetails(t, repo, userB, "Other User Card", nil, nil)

	moved, err := repo.MoveStorageCards(ctx, userA, binder, deckbox)

	require.NoError(t, err)
	assert.Equal(t, 2, moved)
	for _, id := range []int{first.ID, second.ID, stays.ID} {
		c, err := repo.FindByID(ctx, userA, id)
		require.NoError(t, err)
		require.NotNil(t, c.StorageID)
		assert.Equal(t, deckbox, *c.StorageID)
	}
	c, err := repo.FindByID(ctx, userA, loose.ID)
	require.NoError(t, err)
	assert.Nil(t, c.StorageID)
	c, err = repo.FindByID(ctx, userB, foreign.ID)
	require.NoError(t, err)
	assert.Nil(t, c.StorageID)

	moved, err = repo.MoveStorageCards(ctx, userB, binder, deckbox)
	require.NoError(t, err)
	assert.Zero(t, moved)
}

func TestPostgresRepository_MoveStorageCards_ReturnsErrStorageNotFound(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userA)
	seedStorages(t, db, userB)
	ctx := context.Background()

	aliceStorage, bobStorage := 1, 3
	_, err := repo.Create(ctx, userB, Card{Name: "Island", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "1", StorageID: &bobStorage})
	require.NoError(t, err)

	_, err = repo.MoveStorageCards(ctx, userB, bobStorage, aliceStorage)

	assert.ErrorIs(t, err, ErrStorageNotFound)
}
