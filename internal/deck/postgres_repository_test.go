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
		INSERT INTO tamiyo.deck (user_id, name, format, commander_id)
		VALUES
		    ($1, 'Otterly Playful', 'modern', null),
		    ($1, 'Izzet Prowess', 'standard', null);
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

func linkCardToDeck(t *testing.T, db *sqlx.DB, cardID, deckID int) {
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

	linkCardToDeck(t, db, 1, 1)

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

	result, err := repo.FindByID(context.Background(), userID, 1)

	require.NoError(t, err)
	assert.Equal(t, "Otterly Playful", result.Name)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	_, err := repo.FindByID(context.Background(), userID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_FindByID_ReturnsErrNotFoundWhenDeckBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)

	_, err := repo.FindByID(context.Background(), userB, 1)

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

	existing, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)

	existing.Name = "Renamed Deck"
	updated, err := repo.Update(context.Background(), userID, existing)

	require.NoError(t, err)
	assert.Equal(t, "Renamed Deck", updated.Name)

	refetched, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)
	assert.Equal(t, "Renamed Deck", refetched.Name)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenDeckDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	nonExistent := Deck{ID: 999, Name: "Non existent", Format: "modern"}

	_, err := repo.Update(context.Background(), userID, nonExistent)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Update_ReturnsErrNotFoundWhenDeckBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)

	deckFromA := Deck{ID: 1, Name: "Hijacked", Format: "modern"}
	_, err := repo.Update(context.Background(), userB, deckFromA)

	assert.ErrorIs(t, err, ErrNotFound)

	untouched, err := repo.FindByID(context.Background(), userA, 1)
	require.NoError(t, err)
	assert.Equal(t, "Otterly Playful", untouched.Name)
}

func TestPostgresRepository_Update_ReturnsErrCommanderNotFoundOnInvalidCommanderID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	existing, err := repo.FindByID(context.Background(), userID, 1)
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

	existing, err := repo.FindByID(context.Background(), userID, 1)
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

	existing, err := repo.FindByID(context.Background(), userB, 1)
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

	err := repo.Delete(context.Background(), userID, 1)

	require.NoError(t, err)

	_, err = repo.FindByID(context.Background(), userID, 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_ReturnsErrNotFoundWhenDeckDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	err := repo.Delete(context.Background(), userID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_Delete_DoesNotAffectAnotherUsersDeck(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)

	err := repo.Delete(context.Background(), userB, 1)

	assert.ErrorIs(t, err, ErrNotFound)

	result, err := repo.FindByID(context.Background(), userA, 1)
	require.NoError(t, err)
	assert.Equal(t, "Otterly Playful", result.Name)
}

func TestPostgresRepository_FindCardsByDeckID_ReturnsCardsInDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	linkCardToDeck(t, db, 1, 1)
	linkCardToDeck(t, db, 2, 1)

	result, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "updated", true)

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
	_, err := db.Exec(`UPDATE tamiyo.cards SET colors = 'R', card_type = 'Instant' WHERE id = 2`)
	require.NoError(t, err)

	linkCardToDeck(t, db, 1, 1)
	linkCardToDeck(t, db, 2, 1)

	result, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "name", false)

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "Black Lotus", result[0].Name)
	assert.Nil(t, result[0].Colors)
	assert.Nil(t, result[0].CardType)
	require.NotNil(t, result[1].Colors)
	assert.Equal(t, "R", *result[1].Colors)
	require.NotNil(t, result[1].CardType)
	assert.Equal(t, "Instant", *result[1].CardType)
}

func TestPostgresRepository_FindCardsByDeckID_ReturnsEmptySliceWhenDeckHasNoCards(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	result, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "updated", true)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestPostgresRepository_FindCardsByDeckID_OnlyReturnsCardsFromRequestedDeck(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	linkCardToDeck(t, db, 1, 1)
	linkCardToDeck(t, db, 2, 2)
	linkCardToDeck(t, db, 3, 2)

	result, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "updated", true)

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

	linkCardToDeck(t, db, 1, 1) // Black Lotus, mana_value 0
	linkCardToDeck(t, db, 2, 1) // Lightning Bolt, mana_value 1
	linkCardToDeck(t, db, 3, 1) // Counterspell, mana_value 2

	ascending, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "mana_value", false)
	require.NoError(t, err)
	require.Len(t, ascending, 3)
	assert.Equal(t, []string{"Black Lotus", "Lightning Bolt", "Counterspell"}, []string{ascending[0].Name, ascending[1].Name, ascending[2].Name})

	descending, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "mana_value", true)
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

	err := repo.LinkCardToDeck(context.Background(), userID, 1, 1)

	require.NoError(t, err)

	cards, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "updated", true)
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

	err1 := repo.LinkCardToDeck(context.Background(), userID, 1, 1)
	require.NoError(t, err1)

	err2 := repo.LinkCardToDeck(context.Background(), userID, 1, 1)
	require.NoError(t, err2)

	cards, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "updated", true)
	require.NoError(t, err)
	assert.Len(t, cards, 1)
}

func TestPostgresRepository_LinkCardToDeck_ReturnsErrCardNotFoundOnInvalidCardID(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)

	err := repo.LinkCardToDeck(context.Background(), userID, 1, 9999)

	assert.ErrorIs(t, err, ErrCardNotFound)
}

func TestPostgresRepository_LinkCardToDeck_ReturnsErrCardNotFoundWhenCardBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)
	seedCardsWithoutStorage(t, db, userB)

	err := repo.LinkCardToDeck(context.Background(), userA, 1, 1)

	assert.ErrorIs(t, err, ErrCardNotFound)
}

func TestPostgresRepository_LinkCardToDeck_DoesNotAffectOtherDecks(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	err := repo.LinkCardToDeck(context.Background(), userID, 1, 1)
	require.NoError(t, err)

	cardsInDeck2, err := repo.FindCardsByDeckID(context.Background(), userID, 2, "updated", true)
	require.NoError(t, err)
	assert.Empty(t, cardsInDeck2)
}

func TestPostgresRepository_UnlinkCardFromDeck_RemovesLink(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)
	linkCardToDeck(t, db, 1, 1)

	err := repo.UnlinkCardFromDeck(context.Background(), userID, 1, 1)

	require.NoError(t, err)

	cards, err := repo.FindCardsByDeckID(context.Background(), userID, 1, "updated", true)
	require.NoError(t, err)
	assert.Empty(t, cards)
}

func TestPostgresRepository_UnlinkCardFromDeck_SucceedsWhenLinkDoesNotExist(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	err := repo.UnlinkCardFromDeck(context.Background(), userID, 1, 1)

	assert.NoError(t, err)
}

func TestPostgresRepository_UnlinkCardFromDeck_DoesNotRemoveLinkWhenDeckBelongsToAnotherUser(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userA)
	seedCardsWithoutStorage(t, db, userA)
	linkCardToDeck(t, db, 1, 1)

	err := repo.UnlinkCardFromDeck(context.Background(), userB, 1, 1)
	require.NoError(t, err) // idempotent, pas d'erreur, mais rien ne doit changer

	cards, err := repo.FindCardsByDeckID(context.Background(), userA, 1, "updated", true)
	require.NoError(t, err)
	require.Len(t, cards, 1)
}

func TestPostgresRepository_CardLinks_RefreshUpdatedTimestamp(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedDecks(t, db, userID)
	seedCardsWithoutStorage(t, db, userID)

	before, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)
	other, err := repo.FindByID(context.Background(), userID, 2)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.LinkCardToDeck(context.Background(), userID, 1, 1))

	afterLink, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)
	assert.True(t, afterLink.Updated.After(before.Updated))
	untouched, err := repo.FindByID(context.Background(), userID, 2)
	require.NoError(t, err)
	assert.Equal(t, other.Updated, untouched.Updated)

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.UnlinkCardFromDeck(context.Background(), userID, 1, 1))

	afterUnlink, err := repo.FindByID(context.Background(), userID, 1)
	require.NoError(t, err)
	assert.True(t, afterUnlink.Updated.After(afterLink.Updated))
}
