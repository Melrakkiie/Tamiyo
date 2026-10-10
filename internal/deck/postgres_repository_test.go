//go:build integration

package deck

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

func seedDecks(t *testing.T, db *sqlx.DB, userID string) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO tamiyo.deck (id, user_id, name, format, commander_id)
		VALUES
		    ('00000000-0000-0000-0000-000000000001', $1, 'Otterly Playful', 'modern', null),
		    ('00000000-0000-0000-0000-000000000002', $1, 'Izzet Prowess', 'standard', null);
	`, userID)
	require.NoError(t, err)
}

func seedCardsWithoutStorage(t *testing.T, db *sqlx.DB, userID string) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, storage_id, mana_value)
		VALUES
		    ($1, 'Black Lotus', 'bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd', 'lea', '232', false, null, 0),
		    ($1, 'Lightning Bolt', '9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d', '2xm', '129', true, null, 1),
		    ($1, 'Counterspell', '1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f', 'mh2', '267', false, null, 2);
	`, userID)
	require.NoError(t, err)
}

func linkCardToDeck(t *testing.T, db *sqlx.DB, cardID int, deckID string) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO tamiyo.card_deck (card_id, deck_id)
		VALUES ($1, $2)
	`, cardID, deckID)
	require.NoError(t, err)
}

func defaultFilter() Filter {
	return Filter{Page: 1, Limit: 25}
}

func TestPostgresRepository_FindAll_ReturnsAllDecks(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	result, total, err := repo.FindAll(context.Background(), userID, defaultFilter())

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 2, total)
}

func TestPostgresRepository_FindAll_DoesNotReturnOtherUsersDecks(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)

	result, total, err := repo.FindAll(context.Background(), userB, defaultFilter())

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Zero(t, total)
}

func TestPostgresRepository_FindAll_ReturnsCorrectCardCount(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	linkCardToDeck(t, db, 1, "00000000-0000-0000-0000-000000000001")

	result, _, err := repo.FindAll(context.Background(), userID, defaultFilter())

	require.NoError(t, err)
	require.Len(t, result, 2)

	var otters Deck
	for _, d := range result {
		if d.Name == "Otterly Playful" {
			otters = d
		}
	}
	assert.Equal(t, 1, otters.CardCount)
}

func TestPostgresRepository_FindAll_FiltersByFormat(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID) // 'Otterly Playful'/modern, 'Izzet Prowess'/standard

	result, total, err := repo.FindAll(context.Background(), userID, Filter{Format: "modern", Page: 1, Limit: 25})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, "Otterly Playful", result[0].Name)
}

func TestPostgresRepository_FindAll_FormatFilterIsCaseInsensitive(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	result, _, err := repo.FindAll(context.Background(), userID, Filter{Format: "MODERN", Page: 1, Limit: 25})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Otterly Playful", result[0].Name)
}

func TestPostgresRepository_FindAll_FormatFilterReturnsEmptyWhenNoMatch(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	result, total, err := repo.FindAll(context.Background(), userID, Filter{Format: "legacy", Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Zero(t, total)
}

func TestPostgresRepository_FindAll_ReturnsOnlyOnePageAtATime(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID) // 2 decks total

	result, total, err := repo.FindAll(context.Background(), userID, Filter{Page: 1, Limit: 1})

	require.NoError(t, err)
	assert.Len(t, result, 1, "limit must cap the page size")
	assert.Equal(t, 2, total, "total must reflect all matching rows, not just this page")
}

func TestPostgresRepository_FindAll_ReturnsSecondPage(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID) // 2 decks total

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
	seedDecks(t, db, userID) // 2 decks total

	result, total, err := repo.FindAll(context.Background(), userID, Filter{Page: 3, Limit: 25})

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 2, total)
}

func TestPostgresRepository_FindByID_ReturnsDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	result, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")

	require.NoError(t, err)
	assert.Equal(t, "Otterly Playful", result.Name)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000999")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenDeckBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)

	_, err := repo.FindByID(context.Background(), userB, "00000000-0000-0000-0000-000000000001")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Create_InsertsAndReturnsDeckWithID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	newDeck := Deck{Name: "Otterly Playful", Format: "commander"}

	created, err := repo.Create(context.Background(), userID, newDeck)

	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "Otterly Playful", created.Name)

	all, _, err := repo.FindAll(context.Background(), userID, defaultFilter())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, created.ID, all[0].ID)
}

func TestPostgresRepository_StoresBackgroundScryfallID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	background := "436d6a84-4cea-4ca7-94aa-9d08280652af"
	created, err := repo.Create(context.Background(), userID, Deck{Name: "Otterly Playful", Format: "commander", BackgroundScryfallID: &background})
	require.NoError(t, err)
	require.NotNil(t, created.BackgroundScryfallID)
	assert.Equal(t, background, *created.BackgroundScryfallID)

	found, err := repo.FindByID(context.Background(), userID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, found.BackgroundScryfallID)
	assert.Equal(t, background, *found.BackgroundScryfallID)

	all, _, err := repo.FindAll(context.Background(), userID, defaultFilter())
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.NotNil(t, all[0].BackgroundScryfallID)

	found.BackgroundScryfallID = nil
	updated, err := repo.Update(context.Background(), userID, found)
	require.NoError(t, err)
	assert.Nil(t, updated.BackgroundScryfallID)
}

func TestPostgresRepository_Create_GeneratesAddedAndUpdatedTimestamps(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	before := time.Now()
	newDeck := Deck{Name: "Otterly Playful", Format: "commander"}

	created, err := repo.Create(context.Background(), userID, newDeck)
	after := time.Now()

	require.NoError(t, err)
	assert.WithinRange(t, created.Added, before, after)
	assert.WithinRange(t, created.Updated, before, after)
}

func TestPostgresRepository_Create_ReturnsErrCommanderNotFoundOnInvalidCommanderID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	invalidCommanderID := 9999
	newDeck := Deck{Name: "Otterly Playful", Format: "commander", CommanderID: &invalidCommanderID}

	_, err := repo.Create(context.Background(), userID, newDeck)

	assert.ErrorIs(t, err, ErrCommanderNotFound)
}

func TestPostgresRepository_Create_ReturnsErrCommanderNotFoundWhenCardBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedCardsWithoutStorage(t, db, userA)

	commanderID := 1
	newDeck := Deck{Name: "Kess Commander", Format: "commander", CommanderID: &commanderID}

	_, err := repo.Create(context.Background(), userB, newDeck)

	assert.ErrorIs(t, err, ErrCommanderNotFound)
}

func TestPostgresRepository_Update_UpdatesAndReturnsDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	existing, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)

	existing.Name = "Renamed Deck"
	updated, err := repo.Update(context.Background(), userID, existing)

	require.NoError(t, err)
	assert.Equal(t, "Renamed Deck", updated.Name)

	refetched, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Equal(t, "Renamed Deck", refetched.Name)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenDeckDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	nonExistent := Deck{ID: "00000000-0000-0000-0000-000000000999", Name: "Non existent", Format: "modern"}

	_, err := repo.Update(context.Background(), userID, nonExistent)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenDeckBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)

	deckFromA := Deck{ID: "00000000-0000-0000-0000-000000000001", Name: "Hijacked", Format: "modern"}
	_, err := repo.Update(context.Background(), userB, deckFromA)

	assert.ErrorIs(t, err, ErrNotFound)

	untouched, err := repo.FindByID(context.Background(), userA, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Equal(t, "Otterly Playful", untouched.Name)
}

func TestPostgresRepository_Update_ReturnsErrCommanderNotFoundOnInvalidCommanderID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	existing, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)

	invalidCommanderID := 9999
	existing.CommanderID = &invalidCommanderID
	_, errUpdate := repo.Update(context.Background(), userID, existing)

	assert.ErrorIs(t, errUpdate, ErrCommanderNotFound)
}

func TestPostgresRepository_Update_RefreshesUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	existing, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	existing.Name = "Renamed"
	updated, err := repo.Update(context.Background(), userID, existing)

	require.NoError(t, err)
	assert.True(t, updated.Updated.After(existing.Updated))
}

func TestPostgresRepository_Update_ReturnsErrCommanderNotFoundWhenCardBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userB)
	seedCardsWithoutStorage(t, db, userA)

	existing, err := repo.FindByID(context.Background(), userB, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)

	commanderID := 1
	existing.CommanderID = &commanderID
	_, err = repo.Update(context.Background(), userB, existing)

	assert.ErrorIs(t, err, ErrCommanderNotFound)
}

func TestPostgresRepository_Delete_RemovesDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	err := repo.Delete(context.Background(), userID, "00000000-0000-0000-0000-000000000001")

	require.NoError(t, err)

	_, err = repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_ReturnsErrNotFoundWhenDeckDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	err := repo.Delete(context.Background(), userID, "00000000-0000-0000-0000-000000000999")

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_DoesNotAffectAnotherUsersDeck(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)

	err := repo.Delete(context.Background(), userB, "00000000-0000-0000-0000-000000000001")

	assert.ErrorIs(t, err, ErrNotFound)

	result, err := repo.FindByID(context.Background(), userA, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Equal(t, "Otterly Playful", result.Name)
}

func TestPostgresRepository_FindCardsByDeckID_ReturnsCardsInDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	linkCardToDeck(t, db, 1, "00000000-0000-0000-0000-000000000001")
	linkCardToDeck(t, db, 2, "00000000-0000-0000-0000-000000000001")

	result, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "updated", true)

	require.NoError(t, err)
	require.Len(t, result, 2)

	names := []string{result[0].Name, result[1].Name}
	assert.Contains(t, names, "Black Lotus")
	assert.Contains(t, names, "Lightning Bolt")
}

func TestPostgresRepository_FindCardsByDeckID_ReturnsColorsAndType(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)
	_, err := db.Exec(`UPDATE tamiyo.cards SET colors = 'R', card_type = 'Instant', color_identity = 'R' WHERE id = 2`)
	require.NoError(t, err)

	linkCardToDeck(t, db, 1, "00000000-0000-0000-0000-000000000001")
	linkCardToDeck(t, db, 2, "00000000-0000-0000-0000-000000000001")

	result, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "name", false)

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "Black Lotus", result[0].Name)
	assert.Nil(t, result[0].Colors)
	assert.Nil(t, result[0].CardType)
	require.NotNil(t, result[1].Colors)
	assert.Equal(t, "R", *result[1].Colors)
	require.NotNil(t, result[1].CardType)
	assert.Equal(t, "Instant", *result[1].CardType)
	require.NotNil(t, result[1].ColorIdentity)
	assert.Equal(t, "R", *result[1].ColorIdentity)
	assert.Nil(t, result[0].ColorIdentity)
}

func TestPostgresRepository_FindCardsByDeckID_ReturnsEmptySliceWhenDeckHasNoCards(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	result, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "updated", true)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestPostgresRepository_FindCardsByDeckID_OnlyReturnsCardsFromRequestedDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	linkCardToDeck(t, db, 1, "00000000-0000-0000-0000-000000000001")
	linkCardToDeck(t, db, 2, "00000000-0000-0000-0000-000000000002")
	linkCardToDeck(t, db, 3, "00000000-0000-0000-0000-000000000002")

	result, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "updated", true)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Black Lotus", result[0].Name)
}

func TestPostgresRepository_FindCardsByDeckID_SortsByManaValue(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	linkCardToDeck(t, db, 1, "00000000-0000-0000-0000-000000000001") // Black Lotus, mana_value 0
	linkCardToDeck(t, db, 2, "00000000-0000-0000-0000-000000000001") // Lightning Bolt, mana_value 1
	linkCardToDeck(t, db, 3, "00000000-0000-0000-0000-000000000001") // Counterspell, mana_value 2

	ascending, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "mana_value", false)
	require.NoError(t, err)
	require.Len(t, ascending, 3)
	assert.Equal(t, []string{"Black Lotus", "Lightning Bolt", "Counterspell"}, []string{ascending[0].Name, ascending[1].Name, ascending[2].Name})

	descending, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "mana_value", true)
	require.NoError(t, err)
	require.Len(t, descending, 3)
	assert.Equal(t, []string{"Counterspell", "Lightning Bolt", "Black Lotus"}, []string{descending[0].Name, descending[1].Name, descending[2].Name})
}

func TestPostgresRepository_LinkCardToDeck_CreatesLink(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	err := repo.LinkCardToDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 1, BoardMain)

	require.NoError(t, err)

	cards, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "updated", true)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "Black Lotus", cards[0].Name)
}

func TestPostgresRepository_LinkCardToDeck_IsIdempotent(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	err1 := repo.LinkCardToDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 1, BoardMain)
	require.NoError(t, err1)

	err2 := repo.LinkCardToDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 1, BoardMain)
	require.NoError(t, err2)

	cards, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "updated", true)
	require.NoError(t, err)
	assert.Len(t, cards, 1)
}

func TestPostgresRepository_LinkCardToDeck_ReturnsErrCardNotFoundOnInvalidCardID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	err := repo.LinkCardToDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 9999, BoardMain)

	assert.ErrorIs(t, err, ErrCardNotFound)
}

func TestPostgresRepository_LinkCardToDeck_ReturnsErrCardNotFoundWhenCardBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)
	seedCardsWithoutStorage(t, db, userB)

	err := repo.LinkCardToDeck(context.Background(), userA, "00000000-0000-0000-0000-000000000001", 1, BoardMain)

	assert.ErrorIs(t, err, ErrCardNotFound)
}

func TestPostgresRepository_LinkCardToDeck_DoesNotAffectOtherDecks(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	err := repo.LinkCardToDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 1, BoardMain)
	require.NoError(t, err)

	cardsInDeck2, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000002", "updated", true)
	require.NoError(t, err)
	assert.Empty(t, cardsInDeck2)
}

func TestPostgresRepository_UnlinkCardFromDeck_RemovesLink(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)
	linkCardToDeck(t, db, 1, "00000000-0000-0000-0000-000000000001")

	err := repo.UnlinkCardFromDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 1)

	require.NoError(t, err)

	cards, err := repo.FindCardsByDeckID(context.Background(), userID, "00000000-0000-0000-0000-000000000001", "updated", true)
	require.NoError(t, err)
	assert.Empty(t, cards)
}

func TestPostgresRepository_UnlinkCardFromDeck_SucceedsWhenLinkDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	err := repo.UnlinkCardFromDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 1)

	assert.NoError(t, err)
}

func TestPostgresRepository_UnlinkCardFromDeck_DoesNotRemoveLinkWhenDeckBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)
	seedCardsWithoutStorage(t, db, userA)
	linkCardToDeck(t, db, 1, "00000000-0000-0000-0000-000000000001")

	err := repo.UnlinkCardFromDeck(context.Background(), userB, "00000000-0000-0000-0000-000000000001", 1)
	require.NoError(t, err) // idempotent, pas d'erreur, mais rien ne doit changer

	cards, err := repo.FindCardsByDeckID(context.Background(), userA, "00000000-0000-0000-0000-000000000001", "updated", true)
	require.NoError(t, err)
	require.Len(t, cards, 1)
}

func TestPostgresRepository_CardLinks_RefreshUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	before, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	other, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000002")
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.LinkCardToDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 1, BoardMain))

	afterLink, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.True(t, afterLink.Updated.After(before.Updated))
	untouched, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000002")
	require.NoError(t, err)
	assert.Equal(t, other.Updated, untouched.Updated)

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.UnlinkCardFromDeck(context.Background(), userID, "00000000-0000-0000-0000-000000000001", 1))

	afterUnlink, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.True(t, afterUnlink.Updated.After(afterLink.Updated))
}

func TestPostgresRepository_ReturnsCommanderScryfallID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedCardsWithoutStorage(t, db, userID)

	commanderID := 1
	created, err := repo.Create(context.Background(), userID, Deck{Name: "Lotus", Format: "commander", CommanderID: &commanderID})
	require.NoError(t, err)
	require.NotNil(t, created.CommanderScryfallID)
	assert.Equal(t, "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", *created.CommanderScryfallID)

	found, err := repo.FindByID(context.Background(), userID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, found.CommanderScryfallID)
	assert.Equal(t, "bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd", *found.CommanderScryfallID)

	all, _, err := repo.FindAll(context.Background(), userID, defaultFilter())
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.NotNil(t, all[0].CommanderScryfallID)

	found.CommanderID = nil
	updated, err := repo.Update(context.Background(), userID, found)
	require.NoError(t, err)
	assert.Nil(t, updated.CommanderScryfallID)
}

func TestPostgresRepository_ReturnsColorIdentity(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedCardsWithoutStorage(t, db, userID)
	_, err := db.Exec(`UPDATE tamiyo.cards SET color_identity = CASE id WHEN 1 THEN 'gwu' WHEN 2 THEN 'R' ELSE 'B' END`)
	require.NoError(t, err)

	commanderID := 1
	created, err := repo.Create(context.Background(), userID, Deck{Name: "Lotus", Format: "commander", CommanderID: &commanderID})
	require.NoError(t, err)
	require.NotNil(t, created.ColorIdentity)
	assert.Equal(t, "WUG", *created.ColorIdentity)

	require.NoError(t, repo.LinkCardToDeck(context.Background(), userID, created.ID, 2, BoardMain))
	found, err := repo.FindByID(context.Background(), userID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, found.ColorIdentity)
	assert.Equal(t, "WUG", *found.ColorIdentity)

	pending, err := repo.CreatePendingCard(context.Background(), userID, PendingCard{
		DeckID: created.ID, Name: "Karn", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "dmu", CollectorNumber: "1", Quantity: 1,
	})
	require.NoError(t, err)
	found.CommanderID = nil
	found.CommanderPendingID = &pending.ID
	updated, err := repo.Update(context.Background(), userID, found)
	require.NoError(t, err)
	require.NotNil(t, updated.ColorIdentity)
	assert.Equal(t, "", *updated.ColorIdentity)

	identity := "u"
	_, err = repo.CreatePendingCard(context.Background(), userID, PendingCard{
		DeckID: created.ID, Name: "Opt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1e", SetCode: "xln", CollectorNumber: "65", Quantity: 1, ColorIdentity: &identity,
	})
	require.NoError(t, err)
	require.NoError(t, repo.LinkCardToDeck(context.Background(), userID, created.ID, 3, BoardSideboard))
	updated.CommanderPendingID = nil
	cleared, err := repo.Update(context.Background(), userID, updated)
	require.NoError(t, err)
	require.NotNil(t, cleared.ColorIdentity)
	assert.Equal(t, "UR", *cleared.ColorIdentity)

	empty, err := repo.Create(context.Background(), userID, Deck{Name: "Vide", Format: "modern"})
	require.NoError(t, err)
	assert.Nil(t, empty.ColorIdentity)

	all, _, err := repo.FindAll(context.Background(), userID, Filter{Page: 1, Limit: 25, SortField: "name"})
	require.NoError(t, err)
	identities := map[string]*string{}
	for _, d := range all {
		identities[d.Name] = d.ColorIdentity
	}
	require.NotNil(t, identities["Lotus"])
	assert.Equal(t, "UR", *identities["Lotus"])
	assert.Nil(t, identities["Vide"])
}

func TestPostgresRepository_PendingCards(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, alice)

	colors := "R"
	created, err := repo.CreatePendingCard(context.Background(), alice, PendingCard{
		DeckID: "00000000-0000-0000-0000-000000000001", Name: "Lightning Bolt", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d",
		SetCode: "2xm", CollectorNumber: "129", Quantity: 2, ManaValue: 1, Colors: &colors,
	})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, 2, created.Quantity)
	require.NotNil(t, created.Colors)
	assert.Equal(t, "R", *created.Colors)

	_, err = repo.CreatePendingCard(context.Background(), alice, PendingCard{
		DeckID: "00000000-0000-0000-0000-000000000001", Name: "Abrade", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1e", SetCode: "dmu", CollectorNumber: "116", Quantity: 1,
	})
	require.NoError(t, err)

	found, err := repo.FindPendingCards(context.Background(), alice, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	require.Len(t, found, 2)
	assert.Equal(t, "Abrade", found[0].Name)

	others, err := repo.FindPendingCards(context.Background(), bob, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Empty(t, others)

	assert.ErrorIs(t, repo.DeletePendingCard(context.Background(), bob, "00000000-0000-0000-0000-000000000001", created.ID), ErrPendingCardNotFound)
	require.NoError(t, repo.DeletePendingCard(context.Background(), alice, "00000000-0000-0000-0000-000000000001", created.ID))
	assert.ErrorIs(t, repo.DeletePendingCard(context.Background(), alice, "00000000-0000-0000-0000-000000000001", created.ID), ErrPendingCardNotFound)

	require.NoError(t, repo.Delete(context.Background(), alice, "00000000-0000-0000-0000-000000000001"))
	found, err = repo.FindPendingCards(context.Background(), alice, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestPostgresRepository_CountsPendingCopies(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	for _, quantity := range []int{2, 1} {
		_, err := repo.CreatePendingCard(context.Background(), userID, PendingCard{
			DeckID: "00000000-0000-0000-0000-000000000001", Name: "Sol Ring", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "c21", CollectorNumber: "263", Quantity: quantity,
		})
		require.NoError(t, err)
	}

	found, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Equal(t, 3, found.PendingCount)

	all, _, err := repo.FindAll(context.Background(), userID, Filter{Page: 1, Limit: 25, SortField: "name"})
	require.NoError(t, err)
	counts := map[string]int{}
	for _, d := range all {
		counts[d.ID] = d.PendingCount
	}
	assert.Equal(t, map[string]int{"00000000-0000-0000-0000-000000000001": 3, "00000000-0000-0000-0000-000000000002": 0}, counts)
}

func TestPostgresRepository_PendingCommander(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	pending, err := repo.CreatePendingCard(context.Background(), userID, PendingCard{
		DeckID: "00000000-0000-0000-0000-000000000001", Name: "Atraxa", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "2xm", CollectorNumber: "190", Quantity: 1,
	})
	require.NoError(t, err)

	deck, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	deck.CommanderPendingID = &pending.ID
	updated, err := repo.Update(context.Background(), userID, deck)
	require.NoError(t, err)
	require.NotNil(t, updated.CommanderPendingID)
	assert.Equal(t, pending.ID, *updated.CommanderPendingID)
	require.NotNil(t, updated.CommanderScryfallID)
	assert.Equal(t, "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", *updated.CommanderScryfallID)

	require.NoError(t, repo.DeletePendingCard(context.Background(), userID, "00000000-0000-0000-0000-000000000001", pending.ID))
	found, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Nil(t, found.CommanderPendingID)
	assert.Nil(t, found.CommanderScryfallID)
}

func TestPostgresRepository_DeletingTheCommanderCardKeepsItAsPendingCommander(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)
	linkCardToDeck(t, db, 1, "00000000-0000-0000-0000-000000000001")

	deck, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	commanderID := 1
	deck.CommanderID = &commanderID
	_, err = repo.Update(context.Background(), userID, deck)
	require.NoError(t, err)

	_, err = db.Exec(`DELETE FROM tamiyo.cards WHERE id = 1`)
	require.NoError(t, err)

	found, err := repo.FindByID(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	assert.Nil(t, found.CommanderID)
	require.NotNil(t, found.CommanderPendingID)
	pending, err := repo.FindPendingCards(context.Background(), userID, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, pending[0].ID, *found.CommanderPendingID)
	assert.Equal(t, "Black Lotus", pending[0].Name)
}

func TestPostgresRepository_Visibility_DefaultsToUnlistedAndCanBeChanged(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), userID, Deck{Name: "Otters", Format: "commander"})
	require.NoError(t, err)
	assert.Equal(t, VisibilityUnlisted, created.Visibility)

	created.Visibility = VisibilityPublic
	updated, err := repo.Update(context.Background(), userID, created)
	require.NoError(t, err)
	assert.Equal(t, VisibilityPublic, updated.Visibility)

	found, err := repo.FindByID(context.Background(), userID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, VisibilityPublic, found.Visibility)

	decks, _, err := repo.FindAll(context.Background(), userID, Filter{Page: 1, Limit: 25})
	require.NoError(t, err)
	require.Len(t, decks, 1)
	assert.Equal(t, VisibilityPublic, decks[0].Visibility)
}

func TestPostgresRepository_FindAll_FiltersByVisibility(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	for _, d := range []Deck{
		{Name: "Shown", Format: "commander", Visibility: VisibilityPublic},
		{Name: "Link only", Format: "commander", Visibility: VisibilityUnlisted},
		{Name: "Secret", Format: "commander", Visibility: VisibilityPrivate},
	} {
		_, err := repo.Create(context.Background(), userID, d)
		require.NoError(t, err)
	}

	decks, total, err := repo.FindAll(context.Background(), userID, Filter{Visibility: VisibilityPublic, Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, decks, 1)
	assert.Equal(t, "Shown", decks[0].Name)
}

func TestPostgresRepository_Create_GeneratesARandomUUID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	first, err := repo.Create(context.Background(), userID, Deck{Name: "Otters", Format: "commander"})
	require.NoError(t, err)
	second, err := repo.Create(context.Background(), userID, Deck{Name: "Birds", Format: "commander"})
	require.NoError(t, err)
	_, valid := ParseID(first.ID)
	assert.True(t, valid)
	assert.NotEqual(t, first.ID, second.ID)

	first.Name = "Sea otters"
	updated, err := repo.Update(context.Background(), userID, first)
	require.NoError(t, err)
	assert.Equal(t, first.ID, updated.ID)

	found, err := repo.FindByID(context.Background(), userID, first.ID)
	require.NoError(t, err)
	assert.Equal(t, "Sea otters", found.Name)
}

func TestPostgresRepository_FindShared_ReturnsPublicAndUnlistedDecksWithTheirOwner(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	for _, visibility := range []string{VisibilityPublic, VisibilityUnlisted} {
		created, err := repo.Create(context.Background(), userID, Deck{Name: "Otters", Format: "commander", Visibility: visibility})
		require.NoError(t, err)

		ownerID, found, err := repo.FindShared(context.Background(), created.ID)

		require.NoError(t, err)
		assert.Equal(t, userID, ownerID)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, visibility, found.Visibility)
	}
}

func TestPostgresRepository_FindShared_HidesPrivateAndUnknownDecks(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	created, err := repo.Create(context.Background(), userID, Deck{Name: "Secret", Format: "commander", Visibility: VisibilityPrivate})
	require.NoError(t, err)

	_, _, err = repo.FindShared(context.Background(), created.ID)
	assert.ErrorIs(t, err, ErrNotFound)

	_, _, err = repo.FindShared(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_UpdatePendingQuantity(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	otherID := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	d, err := repo.Create(context.Background(), userID, Deck{Name: "Islands", Format: "commander"})
	require.NoError(t, err)
	created, err := repo.CreatePendingCard(context.Background(), userID, PendingCard{
		DeckID: d.ID, Name: "Island", ScryfallID: "11111111-1111-1111-1111-111111111111", SetCode: "mom", CollectorNumber: "278", Quantity: 34,
	})
	require.NoError(t, err)

	updated, err := repo.UpdatePendingQuantity(context.Background(), userID, d.ID, created.ID, 24)
	require.NoError(t, err)
	assert.Equal(t, 24, updated.Quantity)
	assert.Equal(t, "Island", updated.Name)

	_, err = repo.UpdatePendingQuantity(context.Background(), otherID, d.ID, created.ID, 1)
	assert.ErrorIs(t, err, ErrPendingCardNotFound)

	found, err := repo.FindByID(context.Background(), userID, d.ID)
	require.NoError(t, err)
	assert.Equal(t, 24, found.PendingCount)
}

func TestPostgresRepository_CardTags(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	otherID := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	d, err := repo.Create(ctx, userID, Deck{Name: "Kess", Format: "commander"})
	require.NoError(t, err)
	before, err := repo.FindByID(ctx, userID, d.ID)
	require.NoError(t, err)

	require.NoError(t, repo.ReplaceCardTags(ctx, userID, d.ID, "sol ring", []string{"Ramp", "Artefact"}))
	require.NoError(t, repo.ReplaceCardTags(ctx, userID, d.ID, "cultivate", []string{"Ramp"}))
	tags, err := repo.FindCardTags(ctx, userID, d.ID)
	require.NoError(t, err)
	assert.Equal(t, []CardTag{
		{CardName: "cultivate", Tag: "Ramp"},
		{CardName: "sol ring", Tag: "Artefact"},
		{CardName: "sol ring", Tag: "Ramp"},
	}, tags)

	after, err := repo.FindByID(ctx, userID, d.ID)
	require.NoError(t, err)
	assert.True(t, after.Updated.After(before.Updated) || after.Updated.Equal(before.Updated))

	require.NoError(t, repo.ReplaceCardTags(ctx, userID, d.ID, "sol ring", []string{"Mana"}))
	require.NoError(t, repo.RenameTag(ctx, userID, d.ID, "Ramp", "Mana"))
	tags, err = repo.FindCardTags(ctx, userID, d.ID)
	require.NoError(t, err)
	assert.Equal(t, []CardTag{{CardName: "cultivate", Tag: "Mana"}, {CardName: "sol ring", Tag: "Mana"}}, tags)

	require.NoError(t, repo.DeleteTag(ctx, otherID, d.ID, "Mana"))
	assert.ErrorIs(t, repo.ReplaceCardTags(ctx, otherID, d.ID, "sol ring", []string{"Stolen"}), ErrNotFound)
	tags, err = repo.FindCardTags(ctx, otherID, d.ID)
	require.NoError(t, err)
	assert.Empty(t, tags)

	require.NoError(t, repo.DeleteTag(ctx, userID, d.ID, "Mana"))
	tags, err = repo.FindCardTags(ctx, userID, d.ID)
	require.NoError(t, err)
	assert.Empty(t, tags)

	require.NoError(t, repo.ReplaceCardTags(ctx, userID, d.ID, "sol ring", []string{"Ramp"}))
	require.NoError(t, repo.Delete(ctx, userID, d.ID))
	var remaining int
	require.NoError(t, db.Get(&remaining, `SELECT count(*) FROM tamiyo.deck_card_tags`))
	assert.Zero(t, remaining)
}

func TestPostgresRepository_FindPendingCards_CountsOwnedCopiesOutsideTheDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	otherID := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	d, err := repo.Create(ctx, userID, Deck{Name: "Kess", Format: "commander"})
	require.NoError(t, err)
	other, err := repo.Create(ctx, userID, Deck{Name: "Other", Format: "commander"})
	require.NoError(t, err)

	const solRingSLD = "11111111-1111-1111-1111-111111111111"
	const solRingCMM = "22222222-2222-2222-2222-222222222222"
	const solRingC21 = "33333333-3333-3333-3333-333333333333"
	var ids []int
	require.NoError(t, db.Select(&ids, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, storage_id, mana_value)
		VALUES
		    ($1, 'Sol Ring', $3, 'sld', '1011', false, null, 1),
		    ($1, 'Sol Ring', $4, 'cmm', '464', false, null, 1),
		    ($1, 'Sol Ring', $5, 'c21', '263', true, null, 1),
		    ($1, 'Fire // Ice', '44444444-4444-4444-4444-444444444444', 'mh2', '290', false, null, 4),
		    ($2, 'Counterspell', '55555555-5555-5555-5555-555555555555', 'mh2', '267', false, null, 2)
		RETURNING id
	`, userID, otherID, solRingSLD, solRingCMM, solRingC21))
	linkCardToDeck(t, db, ids[1], d.ID)
	linkCardToDeck(t, db, ids[2], other.ID)

	for _, p := range []PendingCard{
		{DeckID: d.ID, Name: "Sol Ring", ScryfallID: solRingSLD, SetCode: "sld", CollectorNumber: "1011", Quantity: 1},
		{DeckID: d.ID, Name: "Fire / Ice", ScryfallID: "66666666-6666-6666-6666-666666666666", SetCode: "uma", CollectorNumber: "225", Quantity: 1},
		{DeckID: d.ID, Name: "Counterspell", ScryfallID: "55555555-5555-5555-5555-555555555555", SetCode: "mh2", CollectorNumber: "267", Quantity: 2},
	} {
		_, err := repo.CreatePendingCard(ctx, userID, p)
		require.NoError(t, err)
	}

	pending, err := repo.FindPendingCards(ctx, userID, d.ID)
	require.NoError(t, err)
	require.Len(t, pending, 3)
	byName := map[string]PendingCard{}
	for _, p := range pending {
		byName[p.Name] = p
	}
	assert.Equal(t, 2, byName["Sol Ring"].OwnedCopies)
	assert.Equal(t, 1, byName["Sol Ring"].OwnedSamePrinting)
	assert.Equal(t, 1, byName["Fire / Ice"].OwnedCopies)
	assert.Equal(t, 0, byName["Fire / Ice"].OwnedSamePrinting)
	assert.Equal(t, 0, byName["Counterspell"].OwnedCopies)
	assert.Equal(t, 0, byName["Counterspell"].OwnedSamePrinting)
}

func TestPostgresRepository_View(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	otherID := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	d, err := repo.Create(ctx, userID, Deck{Name: "Kess", Format: "commander"})
	require.NoError(t, err)
	before, err := repo.FindByID(ctx, userID, d.ID)
	require.NoError(t, err)

	_, found, err := repo.FindView(ctx, userID, d.ID)
	require.NoError(t, err)
	assert.False(t, found)

	grouping := "tag"
	require.NoError(t, repo.SaveView(ctx, userID, d.ID, View{Grouping: &grouping, Sort: "-name", CollapsedBoards: []string{"considering"}}))
	require.NoError(t, repo.SaveView(ctx, userID, d.ID, View{Sort: "name", CollapsedBoards: []string{"sideboard", "considering"}}))
	v, found, err := repo.FindView(ctx, userID, d.ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, View{Sort: "name", CollapsedBoards: []string{"sideboard", "considering"}}, v)

	require.NoError(t, repo.SaveView(ctx, userID, d.ID, View{Sort: "name", CollapsedBoards: []string{}}))
	v, _, err = repo.FindView(ctx, userID, d.ID)
	require.NoError(t, err)
	assert.Empty(t, v.CollapsedBoards)

	after, err := repo.FindByID(ctx, userID, d.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Updated, after.Updated)

	assert.ErrorIs(t, repo.SaveView(ctx, otherID, d.ID, View{Sort: "name", CollapsedBoards: []string{}}), ErrNotFound)
	_, found, err = repo.FindView(ctx, otherID, d.ID)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestPostgresRepository_Boards(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)
	ctx := context.Background()
	deckID := "00000000-0000-0000-0000-000000000001"

	require.NoError(t, repo.LinkCardToDeck(ctx, userID, deckID, 1, BoardMain))
	require.NoError(t, repo.LinkCardToDeck(ctx, userID, deckID, 2, BoardSideboard))
	require.NoError(t, repo.LinkCardToDeck(ctx, userID, deckID, 3, BoardMain))
	require.NoError(t, repo.LinkCardToDeck(ctx, userID, deckID, 3, BoardConsidering))

	main, err := repo.CreatePendingCard(ctx, userID, PendingCard{DeckID: deckID, Name: "Sol Ring", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "c21", CollectorNumber: "263", Quantity: 2})
	require.NoError(t, err)
	assert.Equal(t, BoardMain, main.Board)
	side, err := repo.CreatePendingCard(ctx, userID, PendingCard{DeckID: deckID, Name: "Duress", ScryfallID: "8d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "m19", CollectorNumber: "94", Quantity: 4, Board: BoardSideboard})
	require.NoError(t, err)
	assert.Equal(t, BoardSideboard, side.Board)

	found, err := repo.FindByID(ctx, userID, deckID)
	require.NoError(t, err)
	assert.Equal(t, 1, found.CardCount)
	assert.Equal(t, 2, found.PendingCount)
	_, shared, err := repo.FindShared(ctx, deckID)
	require.NoError(t, err)
	assert.Equal(t, 1, shared.CardCount)
	all, _, err := repo.FindAll(ctx, userID, Filter{Page: 1, Limit: 25, SortField: "name"})
	require.NoError(t, err)
	for _, d := range all {
		if d.ID == deckID {
			assert.Equal(t, 1, d.CardCount)
			assert.Equal(t, 2, d.PendingCount)
		}
	}

	cards, err := repo.FindCardsByDeckID(ctx, userID, deckID, "name", false)
	require.NoError(t, err)
	boards := map[int]string{}
	for _, c := range cards {
		boards[c.ID] = c.Board
	}
	assert.Equal(t, map[int]string{1: BoardMain, 2: BoardSideboard, 3: BoardConsidering}, boards)

	moved, err := repo.UpdatePendingBoard(ctx, userID, deckID, side.ID, BoardConsidering)
	require.NoError(t, err)
	assert.Equal(t, BoardConsidering, moved.Board)
	_, err = repo.UpdatePendingBoard(ctx, userID, deckID, 9999, BoardMain)
	assert.ErrorIs(t, err, ErrPendingCardNotFound)

	_, err = db.Exec(`UPDATE tamiyo.card_deck SET board = 'graveyard' WHERE card_id = 1`)
	assert.Error(t, err)
}

func TestPostgresRepository_MovingACardToAnotherBoardRefreshesTheDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)
	ctx := context.Background()
	deckID := "00000000-0000-0000-0000-000000000001"
	require.NoError(t, repo.LinkCardToDeck(ctx, userID, deckID, 1, BoardMain))
	_, err := db.Exec(`UPDATE tamiyo.deck SET updated = now() - interval '1 day' WHERE id = $1`, deckID)
	require.NoError(t, err)
	before, err := repo.FindByID(ctx, userID, deckID)
	require.NoError(t, err)

	require.NoError(t, repo.LinkCardToDeck(ctx, userID, deckID, 1, BoardSideboard))

	after, err := repo.FindByID(ctx, userID, deckID)
	require.NoError(t, err)
	assert.True(t, after.Updated.After(before.Updated))
}

func TestPostgresRepository_DeletingACardKeepsItsBoardAsPending(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)
	ctx := context.Background()
	deckID := "00000000-0000-0000-0000-000000000001"
	require.NoError(t, repo.LinkCardToDeck(ctx, userID, deckID, 1, BoardSideboard))
	var scryfallID, setCode, collectorNumber string
	require.NoError(t, db.QueryRow(`SELECT scryfall_id, set_code, collector_number FROM tamiyo.cards WHERE id = 1`).Scan(&scryfallID, &setCode, &collectorNumber))
	main, err := repo.CreatePendingCard(ctx, userID, PendingCard{DeckID: deckID, Name: "Black Lotus", ScryfallID: scryfallID, SetCode: setCode, CollectorNumber: collectorNumber, Quantity: 1})
	require.NoError(t, err)

	_, err = db.Exec(`DELETE FROM tamiyo.cards WHERE id = 1`)
	require.NoError(t, err)

	pending, err := repo.FindPendingCards(ctx, userID, deckID)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	byBoard := map[string]PendingCard{}
	for _, p := range pending {
		byBoard[p.Board] = p
	}
	assert.Equal(t, main.ID, byBoard[BoardMain].ID)
	assert.Equal(t, 1, byBoard[BoardMain].Quantity)
	assert.Equal(t, 1, byBoard[BoardSideboard].Quantity)
	assert.Equal(t, "Black Lotus", byBoard[BoardSideboard].Name)
}

func seedPublicCard(t *testing.T, db *sqlx.DB, userID string, name string, identity string) int {
	t.Helper()
	var id int
	require.NoError(t, db.Get(&id, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, color_identity)
		VALUES ($1, $2, gen_random_uuid(), 'tst', '1', false, $3)
		RETURNING id
	`, userID, name, identity))
	return id
}

func publicNames(decks []PublicDeck) []string {
	names := make([]string, 0, len(decks))
	for _, d := range decks {
		names = append(names, d.Name)
	}
	return names
}

func TestPostgresRepository_FindPublic(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	_, err := db.Exec(`UPDATE tamiyo.users SET display_name = 'Alice' WHERE id = $1`, alice)
	require.NoError(t, err)
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	otters, err := repo.Create(ctx, alice, Deck{Name: "Otters", Format: "commander", Visibility: VisibilityPublic})
	require.NoError(t, err)
	loot := seedPublicCard(t, db, alice, "Loot, the Pathfinder", "UG")
	for _, id := range []int{loot, seedPublicCard(t, db, alice, "Sol Ring", ""), seedPublicCard(t, db, alice, "Island", "")} {
		require.NoError(t, repo.LinkCardToDeck(ctx, alice, otters.ID, id, BoardMain))
	}
	require.NoError(t, repo.LinkCardToDeck(ctx, alice, otters.ID, seedPublicCard(t, db, alice, "Lightning Bolt", "R"), BoardSideboard))
	otters.CommanderID = &loot
	_, err = repo.Update(ctx, alice, otters)
	require.NoError(t, err)

	burn, err := repo.Create(ctx, bob, Deck{Name: "Burn", Format: "modern", Visibility: VisibilityPublic})
	require.NoError(t, err)
	require.NoError(t, repo.LinkCardToDeck(ctx, bob, burn.ID, seedPublicCard(t, db, bob, "Lightning Bolt", "R"), BoardMain))
	_, err = repo.CreatePendingCard(ctx, bob, PendingCard{DeckID: burn.ID, Name: "Mountain", ScryfallID: "9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "m21", CollectorNumber: "1", Quantity: 3})
	require.NoError(t, err)
	blue := "U"
	_, err = repo.CreatePendingCard(ctx, bob, PendingCard{DeckID: burn.ID, Name: "Counterspell", ScryfallID: "8d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d", SetCode: "mh2", CollectorNumber: "2", Quantity: 1, ColorIdentity: &blue, Board: BoardConsidering})
	require.NoError(t, err)

	_, err = repo.Create(ctx, alice, Deck{Name: "Hidden otters", Format: "commander", Visibility: VisibilityUnlisted})
	require.NoError(t, err)
	_, err = repo.Create(ctx, bob, Deck{Name: "Private burn", Format: "modern", Visibility: VisibilityPrivate})
	require.NoError(t, err)

	find := func(filter PublicFilter) ([]PublicDeck, int) {
		t.Helper()
		if filter.Page == 0 {
			filter.Page = 1
		}
		if filter.Limit == 0 {
			filter.Limit = 24
		}
		if filter.SortField == "" {
			filter.SortField = "name"
		}
		decks, total, err := repo.FindPublic(ctx, filter)
		require.NoError(t, err)
		return decks, total
	}

	all, total := find(PublicFilter{})
	assert.Equal(t, 2, total)
	require.Equal(t, []string{"Burn", "Otters"}, publicNames(all))
	assert.Equal(t, 4, all[0].CardCount)
	assert.Equal(t, "R", all[0].ColorIdentity)
	assert.Nil(t, all[0].CommanderName)
	assert.Equal(t, 3, all[1].CardCount)
	assert.Equal(t, "UG", all[1].ColorIdentity)
	require.NotNil(t, all[1].CommanderName)
	assert.Equal(t, "Loot, the Pathfinder", *all[1].CommanderName)
	require.NotNil(t, all[1].OwnerDisplayName)
	assert.Equal(t, "Alice", *all[1].OwnerDisplayName)
	assert.Equal(t, alice, all[1].OwnerID)

	two := 2
	cases := map[string]struct {
		filter PublicFilter
		want   []string
	}{
		"name":              {PublicFilter{Name: "OTT"}, []string{"Otters"}},
		"format":            {PublicFilter{Format: "Modern"}, []string{"Burn"}},
		"commander":         {PublicFilter{Commander: "loot"}, []string{"Otters"}},
		"card":              {PublicFilter{Card: "sol"}, []string{"Otters"}},
		"pending card":      {PublicFilter{Card: "mountain"}, []string{"Burn"}},
		"considering card":  {PublicFilter{Card: "counterspell"}, []string{}},
		"sideboard card":    {PublicFilter{Card: "bolt"}, []string{"Burn"}},
		"owner":             {PublicFilter{Owner: "ali"}, []string{"Otters"}},
		"exact colors":      {PublicFilter{Colors: []string{"R"}, ColorMode: ColorModeExact}, []string{"Burn"}},
		"at least a color":  {PublicFilter{Colors: []string{"U"}, ColorMode: ColorModeInclude}, []string{"Otters"}},
		"at most colors":    {PublicFilter{Colors: []string{"U", "G", "R"}, ColorMode: ColorModeWithin}, []string{"Burn", "Otters"}},
		"too few colors":    {PublicFilter{Colors: []string{"G"}, ColorMode: ColorModeWithin}, []string{}},
		"color count":       {PublicFilter{ColorCount: &two}, []string{"Otters"}},
		"colorless":         {PublicFilter{Colorless: true}, []string{}},
		"combined, no hits": {PublicFilter{Format: "commander", Colors: []string{"R"}, ColorMode: ColorModeInclude}, []string{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			decks, total := find(tc.filter)
			assert.Equal(t, tc.want, publicNames(decks))
			assert.Equal(t, len(tc.want), total)
		})
	}

	page, total := find(PublicFilter{Page: 2, Limit: 1, SortField: "card_count", SortDesc: true})
	assert.Equal(t, 2, total)
	assert.Equal(t, []string{"Otters"}, publicNames(page))
	beyond, total := find(PublicFilter{Page: 5, Limit: 1})
	assert.Empty(t, beyond)
	assert.Equal(t, 2, total)
}

func TestPostgresRepository_CountCopiesByName(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedPublicCard(t, db, alice, "Sol Ring", "")
	seedPublicCard(t, db, alice, "Sol Ring", "")
	seedPublicCard(t, db, alice, "Fire // Ice", "UR")
	seedPublicCard(t, db, bob, "Island", "")

	counts, err := repo.CountCopiesByName(context.Background(), alice, []string{CardNameKey("Sol Ring"), CardNameKey("Fire / Ice"), CardNameKey("Island")})

	require.NoError(t, err)
	assert.Equal(t, map[string]int{"sol ring": 2, "fire / ice": 1}, counts)

	empty, err := repo.CountCopiesByName(context.Background(), alice, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestPostgresRepository_Likes(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	carol := seedUser(t, db, "carol@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	public, err := repo.Create(ctx, alice, Deck{Name: "Otters", Format: "commander", Visibility: VisibilityPublic})
	require.NoError(t, err)
	unlisted, err := repo.Create(ctx, alice, Deck{Name: "Birds", Format: "modern", Visibility: VisibilityUnlisted})
	require.NoError(t, err)
	hidden, err := repo.Create(ctx, alice, Deck{Name: "Secret", Format: "modern", Visibility: VisibilityPublic})
	require.NoError(t, err)

	require.NoError(t, repo.Like(ctx, bob, public.ID))
	require.NoError(t, repo.Like(ctx, bob, public.ID))
	require.NoError(t, repo.Like(ctx, carol, public.ID))
	require.NoError(t, repo.Like(ctx, bob, unlisted.ID))
	require.NoError(t, repo.Like(ctx, bob, hidden.ID))
	_, err = db.Exec(`UPDATE tamiyo.deck_likes SET added = now() - interval '1 day' WHERE deck_id = $1`, public.ID)
	require.NoError(t, err)
	hidden.Visibility = VisibilityPrivate
	_, err = repo.Update(ctx, alice, hidden)
	require.NoError(t, err)

	status, err := repo.FindLikeStatus(ctx, bob, public.ID)
	require.NoError(t, err)
	assert.Equal(t, LikeStatus{Count: 2, LikedByMe: true}, status)
	status, err = repo.FindLikeStatus(ctx, alice, public.ID)
	require.NoError(t, err)
	assert.Equal(t, LikeStatus{Count: 2}, status)
	found, err := repo.FindByID(ctx, alice, public.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, found.LikesCount)
	_, shared, err := repo.FindShared(ctx, public.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, shared.LikesCount)
	all, _, err := repo.FindAll(ctx, alice, Filter{Page: 1, Limit: 10})
	require.NoError(t, err)
	for _, d := range all {
		if d.ID == public.ID {
			assert.Equal(t, 2, d.LikesCount)
		}
	}

	liked, total, err := repo.FindLiked(ctx, bob, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, total, "a deck turned private leaves the list")
	require.Len(t, liked, 2)
	assert.Equal(t, unlisted.ID, liked[0].ID, "most recent like first")
	assert.Equal(t, public.ID, liked[1].ID)
	assert.Equal(t, 2, liked[1].LikesCount)
	require.NotNil(t, liked[1].LikedAt)
	assert.Equal(t, alice, liked[1].OwnerID)

	page2, total, err := repo.FindLiked(ctx, bob, 2, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, page2, 1)
	assert.Equal(t, public.ID, page2[0].ID)

	publicDecks, _, err := repo.FindPublic(ctx, PublicFilter{Page: 1, Limit: 10, SortField: "likes", SortDesc: true})
	require.NoError(t, err)
	require.NotEmpty(t, publicDecks)
	assert.Equal(t, public.ID, publicDecks[0].ID)
	assert.Equal(t, 2, publicDecks[0].LikesCount)
	assert.Nil(t, publicDecks[0].LikedAt)

	require.NoError(t, repo.Unlike(ctx, bob, public.ID))
	status, err = repo.FindLikeStatus(ctx, bob, public.ID)
	require.NoError(t, err)
	assert.Equal(t, LikeStatus{Count: 1}, status)

	require.NoError(t, repo.Delete(ctx, alice, unlisted.ID))
	_, total, err = repo.FindLiked(ctx, bob, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

func TestPostgresRepository_Bracket(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	three := 3

	created, err := repo.Create(ctx, alice, Deck{Name: "Otters", Format: "commander", Visibility: VisibilityPublic, Bracket: &three})
	require.NoError(t, err)
	assert.Equal(t, &three, created.Bracket)
	plain, err := repo.Create(ctx, alice, Deck{Name: "Burn", Format: "modern", Visibility: VisibilityPublic})
	require.NoError(t, err)
	assert.Nil(t, plain.Bracket)

	found, err := repo.FindByID(ctx, alice, created.ID)
	require.NoError(t, err)
	assert.Equal(t, &three, found.Bracket)
	all, _, err := repo.FindAll(ctx, alice, Filter{Page: 1, Limit: 10})
	require.NoError(t, err)
	brackets := map[string]*int{}
	for _, d := range all {
		brackets[d.ID] = d.Bracket
	}
	assert.Equal(t, &three, brackets[created.ID])
	assert.Nil(t, brackets[plain.ID])

	public, total, err := repo.FindPublic(ctx, PublicFilter{Page: 1, Limit: 10, Brackets: []int{2, 3}})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, created.ID, public[0].ID)
	assert.Equal(t, &three, public[0].Bracket)

	five := 5
	found.Bracket = &five
	updated, err := repo.Update(ctx, alice, found)
	require.NoError(t, err)
	assert.Equal(t, &five, updated.Bracket)
	_, shared, err := repo.FindShared(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, &five, shared.Bracket)

	updated.Bracket = nil
	cleared, err := repo.Update(ctx, alice, updated)
	require.NoError(t, err)
	assert.Nil(t, cleared.Bracket)

	_, err = db.Exec(`UPDATE tamiyo.deck SET bracket = 6 WHERE id = $1`, created.ID)
	assert.Error(t, err)
}
