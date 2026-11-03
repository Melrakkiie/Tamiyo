//go:build integration

package card

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const importSolRingID = "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f"

func importCopy(name string, storageID *int) ImportCopy {
	return ImportCopy{Card: Card{
		Name: name, ScryfallID: importSolRingID, SetCode: "cmr", CollectorNumber: "322",
		StorageID: storageID, ManaValue: 1,
	}}
}

func countCards(t *testing.T, db interface {
	Get(any, string, ...any) error
}) int {
	t.Helper()
	var n int
	require.NoError(t, db.Get(&n, `SELECT count(*) FROM tamiyo.cards`))
	return n
}

func TestPostgresRepository_ImportCollection_CreatesEveryCopyInInputOrder(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	seedStorages(t, db, userID)
	repo := NewPostgresRepository(db)
	storageID := 1
	colors := "W"

	plan := ImportPlan{}
	names := []string{"Zebra", "Apple", "Mango", "Banana", "Cherry"}
	for _, n := range names {
		plan.Copies = append(plan.Copies, importCopy(n, &storageID))
	}
	plan.Copies[2].Card.Colors = &colors
	plan.Copies[3].Card.Foil = true

	result, err := repo.ImportCollection(context.Background(), userID, plan)

	require.NoError(t, err)
	require.Len(t, result.Cards, len(names))
	for i, n := range names {
		assert.Equal(t, n, result.Cards[i].Name)
		assert.NotZero(t, result.Cards[i].ID)
		assert.Equal(t, &storageID, result.Cards[i].StorageID)
		assert.False(t, result.Cards[i].Added.IsZero())
	}
	assert.Equal(t, &colors, result.Cards[2].Colors)
	assert.True(t, result.Cards[3].Foil)
	assert.Equal(t, len(names), countCards(t, db))
	for _, c := range result.Cards {
		got, err := repo.FindByID(context.Background(), userID, c.ID)
		require.NoError(t, err)
		assert.Equal(t, c.Name, got.Name)
	}
}

func TestPostgresRepository_ImportCollection_SpansSeveralBatches(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)

	plan := ImportPlan{}
	for i := 0; i < importBatchSize*2+3; i++ {
		plan.Copies = append(plan.Copies, importCopy("Sol Ring", nil))
	}

	result, err := repo.ImportCollection(context.Background(), userID, plan)

	require.NoError(t, err)
	assert.Len(t, result.Cards, len(plan.Copies))
	assert.Equal(t, len(plan.Copies), countCards(t, db))
}

func TestPostgresRepository_ImportCollection_WritesNothingWhenOneCopyViolatesAConstraint(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	seedStorages(t, db, userA)
	repo := NewPostgresRepository(db)
	foreign := 1

	_, err := repo.ImportCollection(context.Background(), userB, ImportPlan{Copies: []ImportCopy{
		importCopy("Sol Ring", nil),
		importCopy("Sol Ring", &foreign),
	}})

	assert.ErrorIs(t, err, ErrStorageNotFound)
	assert.Zero(t, countCards(t, db))
}

func TestPostgresRepository_ImportCollection_RollsBackWhenTheContextIsCancelled(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.ImportCollection(ctx, userID, ImportPlan{Copies: []ImportCopy{importCopy("Sol Ring", nil)}})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, countCards(t, db))
}

func TestPostgresRepository_ImportCollection_CreatesTheStoragesOfThePlanAndFillsThem(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	first, second := 0, 1
	plan := ImportPlan{
		Storages: []NewStorage{{Name: "Vrac", Type: "box"}, {Name: "Neuf", Type: "binder"}},
		Copies:   []ImportCopy{importCopy("A", nil), importCopy("B", nil), importCopy("C", nil), importCopy("D", nil)},
	}
	plan.Copies[0].NewStorage, plan.Copies[1].NewStorage, plan.Copies[2].NewStorage = &second, &first, &second

	result, err := repo.ImportCollection(context.Background(), userID, plan)

	require.NoError(t, err)
	require.Len(t, result.StorageIDs, 2)
	assert.Equal(t, result.StorageIDs[1], *result.Cards[0].StorageID)
	assert.Equal(t, result.StorageIDs[0], *result.Cards[1].StorageID)
	assert.Equal(t, result.StorageIDs[1], *result.Cards[2].StorageID)
	assert.Nil(t, result.Cards[3].StorageID)
	var got []struct {
		Name  string `db:"name"`
		Type  string `db:"type"`
		Count int    `db:"count"`
	}
	require.NoError(t, db.Select(&got, `SELECT s.name, s.type, count(c.id) FROM tamiyo.storage s LEFT JOIN tamiyo.cards c ON c.storage_id = s.id GROUP BY s.id ORDER BY s.id`))
	assert.Equal(t, "Vrac", got[0].Name)
	assert.Equal(t, "box", got[0].Type)
	assert.Equal(t, 1, got[0].Count)
	assert.Equal(t, "Neuf", got[1].Name)
	assert.Equal(t, 2, got[1].Count)
}

func TestPostgresRepository_ImportCollection_RollsBackTheCreatedStoragesWhenACopyFails(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	seedStorages(t, db, userA)
	repo := NewPostgresRepository(db)
	idx, foreign := 0, 1
	plan := ImportPlan{
		Storages: []NewStorage{{Name: "Vrac", Type: "box"}},
		Copies:   []ImportCopy{importCopy("A", nil), importCopy("B", &foreign)},
	}
	plan.Copies[0].NewStorage = &idx

	_, err := repo.ImportCollection(context.Background(), userB, plan)

	assert.ErrorIs(t, err, ErrStorageNotFound)
	var n int
	require.NoError(t, db.Get(&n, `SELECT count(*) FROM tamiyo.storage WHERE user_id = $1`, userB))
	assert.Zero(t, n)
	assert.Zero(t, countCards(t, db))
}

func TestPostgresRepository_ImportCollection_CreatesTheDecksOfThePlanAndLinksTheCopiesOnTheMainBoard(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	var existing string
	require.NoError(t, db.Get(&existing, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Old', 'modern') RETURNING id`, userID))
	idx := 0
	plan := ImportPlan{
		Storages: []NewStorage{{Name: "Atraxa", Type: "deck"}},
		Decks:    []NewDeck{{Name: "Atraxa", Format: "commander"}},
		Copies:   []ImportCopy{importCopy("A", nil), importCopy("A", nil), importCopy("B", nil), importCopy("C", nil)},
	}
	plan.Copies[0].NewDeck, plan.Copies[1].NewDeck = &idx, &idx
	plan.Copies[2].DeckID = &existing

	result, err := repo.ImportCollection(context.Background(), userID, plan)

	require.NoError(t, err)
	require.Len(t, result.DeckIDs, 1)
	var links []struct {
		DeckID string `db:"deck_id"`
		Board  string `db:"board"`
		Count  int    `db:"count"`
	}
	require.NoError(t, db.Select(&links, `SELECT deck_id, board, count(*) FROM tamiyo.card_deck GROUP BY deck_id, board ORDER BY count(*) DESC`))
	require.Len(t, links, 2)
	assert.Equal(t, result.DeckIDs[0], links[0].DeckID)
	assert.Equal(t, "main", links[0].Board)
	assert.Equal(t, 2, links[0].Count)
	assert.Equal(t, existing, links[1].DeckID)
	var history int
	require.NoError(t, db.Get(&history, `SELECT coalesce(sum(count), 0) FROM tamiyo.deck_history WHERE deck_id = $1 AND board = 'main'`, result.DeckIDs[0]))
	assert.Equal(t, 2, history)
	var format string
	require.NoError(t, db.Get(&format, `SELECT format FROM tamiyo.deck WHERE id = $1`, result.DeckIDs[0]))
	assert.Equal(t, "commander", format)
}

func TestPostgresRepository_ImportCollection_RollsBackTheCreatedStoragesAndDecksWhenACopyFails(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	seedStorages(t, db, userA)
	repo := NewPostgresRepository(db)
	idx, foreign := 0, 1
	plan := ImportPlan{
		Storages: []NewStorage{{Name: "Atraxa", Type: "deck"}},
		Decks:    []NewDeck{{Name: "Atraxa", Format: "commander"}},
		Copies:   []ImportCopy{importCopy("A", nil), importCopy("B", &foreign)},
	}
	plan.Copies[0].NewDeck = &idx

	_, err := repo.ImportCollection(context.Background(), userB, plan)

	assert.ErrorIs(t, err, ErrStorageNotFound)
	var n int
	require.NoError(t, db.Get(&n, `SELECT count(*) FROM tamiyo.deck WHERE user_id = $1`, userB))
	assert.Zero(t, n)
	assert.Zero(t, countCards(t, db))
}

func TestPostgresRepository_ImportCollection_RejectsADeckOfAnotherUserAndWritesNothing(t *testing.T) {
	db := getTestDB(t)
	userA := seedUser(t, db, "alice@example.com")
	userB := seedUser(t, db, "bob@example.com")
	repo := NewPostgresRepository(db)
	var foreign string
	require.NoError(t, db.Get(&foreign, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Theirs', 'modern') RETURNING id`, userA))
	idx := 0
	plan := ImportPlan{
		Storages: []NewStorage{{Name: "Vrac", Type: "box"}},
		Copies:   []ImportCopy{importCopy("A", nil), importCopy("B", nil)},
	}
	plan.Copies[0].NewStorage = &idx
	plan.Copies[1].DeckID = &foreign

	_, err := repo.ImportCollection(context.Background(), userB, plan)

	assert.ErrorIs(t, err, ErrDeckNotFound)
	var n int
	require.NoError(t, db.Get(&n, `SELECT count(*) FROM tamiyo.card_deck`))
	assert.Zero(t, n)
	require.NoError(t, db.Get(&n, `SELECT count(*) FROM tamiyo.storage WHERE user_id = $1`, userB))
	assert.Zero(t, n)
	assert.Zero(t, countCards(t, db))
}
