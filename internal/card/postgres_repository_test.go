//go:build integration

package card

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
	deadline := time.Now().Add(30 * time.Second)
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

func seedStorages(t *testing.T, db *sqlx.DB) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO tamiyo.storage (name, type)
		VALUES
		    ('Vintage Collection', 'binder'),
		    ('Red Deck Wins', 'deckbox');
	`)
	require.NoError(t, err)
}

func seedCards(t *testing.T, db *sqlx.DB, cards []Card) {
	t.Helper()

	for _, c := range cards {
		_, err := db.Exec(`
			INSERT INTO tamiyo.cards (name, scryfall_id, set_code, collector_number, foil, storage_id)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, c.Name, c.ScryfallID, c.SetCode, c.CollectorNumber, c.Foil, c.StorageID)
		require.NoError(t, err)
	}
}

func TestPostgresRepository_FindAll_ReturnsAllCardsWhenNoFilter(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID1, testID2 := 1, 2
	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: &testID1},
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", Foil: true, StorageID: &testID2},
	})

	result, total, err := repo.FindAll(context.Background(), CardFilter{Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 2, total)
}

func TestPostgresRepository_FindAll_FiltersByStorageID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID1, testID2 := 1, 2
	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: &testID1},
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", Foil: true, StorageID: &testID2},
	})

	testID := 1
	result, total, err := repo.FindAll(context.Background(), CardFilter{StorageID: &testID, Page: 1, Limit: 25})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Black Lotus", result[0].Name)
	assert.Equal(t, 1, total)
}

func TestPostgresRepository_FindAll_ReturnsEmptySliceWhenNoStorageMatches(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID := 1
	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: &testID},
	})

	unknownTestID := 67
	result, total, err := repo.FindAll(context.Background(), CardFilter{StorageID: &unknownTestID, Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 0, total)
}

func TestPostgresRepository_FindAll_FiltersByNameCaseInsensitive(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	seedCards(t, db, []Card{
		{Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "129", Foil: true},
		{Name: "Lightning Helix", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "rav", CollectorNumber: "5", Foil: false},
		{Name: "Counterspell", ScryfallID: "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", SetCode: "mh2", CollectorNumber: "267", Foil: false},
	})

	result, total, err := repo.FindAll(context.Background(), CardFilter{Name: "lightning", Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	names := []string{result[0].Name, result[1].Name}
	assert.Contains(t, names, "Lightning Bolt")
	assert.Contains(t, names, "Lightning Helix")
}

func TestPostgresRepository_FindAll_PaginatesResults(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	for i := 0; i < 5; i++ {
		seedCards(t, db, []Card{
			{
				Name:            "Card " + string(rune('A'+i)),
				ScryfallID:      "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1" + string(rune('0'+i)),
				SetCode:         "test",
				CollectorNumber: "1",
				Foil:            false,
			},
		})
	}

	firstPage, total, err := repo.FindAll(context.Background(), CardFilter{Page: 1, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, firstPage, 2)
	assert.Equal(t, 5, total)

	secondPage, total, err := repo.FindAll(context.Background(), CardFilter{Page: 2, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, secondPage, 2)
	assert.Equal(t, 5, total)

	thirdPage, total, err := repo.FindAll(context.Background(), CardFilter{Page: 3, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, thirdPage, 1)
	assert.Equal(t, 5, total)

	allIDs := map[int]bool{}
	for _, c := range append(append(firstPage, secondPage...), thirdPage...) {
		assert.False(t, allIDs[c.ID], "card ID %d returned on more than one page", c.ID)
		allIDs[c.ID] = true
	}
}

func TestPostgresRepository_FindByID_ReturnsCard(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: nil},
	})

	result, err := repo.FindByID(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, "Black Lotus", result.Name)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Create_InsertsAndReturnsCardWithID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID := 1
	newCard := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: "322",
		Foil:            false,
		StorageID:       &testID,
	}

	created, err := repo.Create(context.Background(), newCard)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Sol Ring", created.Name)

	all, total, err := repo.FindAll(context.Background(), CardFilter{StorageID: &testID, Page: 1, Limit: 25})
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, created.ID, all[0].ID)
}

func TestPostgresRepository_Create_InsertsAndReturnsCardWithID_NoStorageID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	newCard := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: "322",
		Foil:            false,
	}

	created, err := repo.Create(context.Background(), newCard)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Sol Ring", created.Name)
	assert.Nil(t, created.StorageID)

	all, total, err := repo.FindAll(context.Background(), CardFilter{Page: 1, Limit: 25})
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, created.ID, all[0].ID)
}

func TestPostgresRepository_Create_GeneratesAddedAndUpdatedTimestamps(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

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

	created, err := repo.Create(context.Background(), newCard)
	after := time.Now()

	require.NoError(t, err)
	assert.WithinRange(t, created.Added, before, after)
	assert.WithinRange(t, created.Updated, before, after)
}

func TestPostgresRepository_Create_ReturnsErrorOnInvalidScryfallID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	seedStorages(t, db)

	testID := 1
	invalidCard := Card{
		Name:            "Bad Card",
		ScryfallID:      "not-a-uuid",
		SetCode:         "test",
		CollectorNumber: "1",
		Foil:            false,
		StorageID:       &testID,
	}

	_, err := repo.Create(context.Background(), invalidCard)

	assert.Error(t, err)
}

func TestPostgresRepository_Create_ReturnsErrStorageNotFoundOnInvalidStorageID(t *testing.T) {
	db := getTestDB(t)
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

	_, err := repo.Create(context.Background(), newCard)

	assert.ErrorIs(t, err, ErrStorageNotFound)
}

func TestPostgresRepository_Update_UpdatesAndReturnsCard(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: nil},
	})

	existing, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)

	existing.Name = "Renamed Card"
	updated, err := repo.Update(context.Background(), existing)

	require.NoError(t, err)
	assert.Equal(t, "Renamed Card", updated.Name)
	assert.Equal(t, "lea", updated.SetCode)

	refetched, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "Renamed Card", refetched.Name)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenCardDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	nonExistent := Card{ID: 999, Name: "Non existent", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: nil}

	_, err := repo.Update(context.Background(), nonExistent)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_ReturnsErrStorageNotFoundOnInvalidStorageID(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: nil},
	})

	existing, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)

	invalidStorageID := 9999
	existing.Name = "Renamed Card"
	existing.StorageID = &invalidStorageID
	_, errUpdate := repo.Update(context.Background(), existing)

	assert.ErrorIs(t, errUpdate, ErrStorageNotFound)
}

func TestPostgresRepository_Update_RefreshesUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: nil},
	})

	existing, err := repo.FindByID(context.Background(), 1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	existing.Name = "Renamed"
	updated, err := repo.Update(context.Background(), existing)

	require.NoError(t, err)
	assert.True(t, updated.Updated.After(existing.Updated))
}

func TestPostgresRepository_Delete_RemovesCard(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: nil},
	})

	err := repo.Delete(context.Background(), 1)

	require.NoError(t, err)

	_, err = repo.FindByID(context.Background(), 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_ReturnsErrNotFoundWhenCardDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	err := repo.Delete(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_DoesNotAffectOtherCards(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)

	seedCards(t, db, []Card{
		{Name: "Black Lotus", ScryfallID: "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", SetCode: "lea", CollectorNumber: "232", Foil: false, StorageID: nil},
		{Name: "Counterspell", ScryfallID: "1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f", SetCode: "mh2", CollectorNumber: "125", Foil: false, StorageID: nil},
	})

	err := repo.Delete(context.Background(), 1)
	require.NoError(t, err)

	remaining, total, err := repo.FindAll(context.Background(), CardFilter{Page: 1, Limit: 25})
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, "Counterspell", remaining[0].Name)
}
