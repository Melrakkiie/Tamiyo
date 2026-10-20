//go:build integration

package printing

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/scryfall"
	"Melrakkiie/Tamiyo/internal/testdb"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m, &testDB))
}

func getTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	_, err := testDB.Exec(`
		TRUNCATE TABLE tamiyo.cards, tamiyo.users, tamiyo.printings
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

func seedCard(t *testing.T, db *sqlx.DB, userID, scryfallID string) {
	t.Helper()
	seedNamedCard(t, db, userID, scryfallID, "Card")
}

func seedNamedCard(t *testing.T, db *sqlx.DB, userID, scryfallID, name string) int {
	t.Helper()
	var id int
	require.NoError(t, db.Get(&id, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, mana_value)
		VALUES ($1, $2, $3, 'tst', '1', false, 0)
		RETURNING id
	`, userID, name, scryfallID))
	return id
}

func seedDeck(t *testing.T, db *sqlx.DB, userID string, cardIDs ...int) string {
	t.Helper()
	var deckID string
	require.NoError(t, db.Get(&deckID, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Deck', 'commander') RETURNING id`, userID))
	for _, cardID := range cardIDs {
		_, err := db.Exec(`INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES ($1, $2)`, cardID, deckID)
		require.NoError(t, err)
	}
	return deckID
}

func seedPendingCard(t *testing.T, db *sqlx.DB, userID, deckID, scryfallID, name string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO tamiyo.deck_pending_cards (user_id, deck_id, name, scryfall_id, set_code, collector_number)
		VALUES ($1, $2, $3, $4, 'tst', '1')
	`, userID, deckID, name, scryfallID)
	require.NoError(t, err)
}

func seedTag(t *testing.T, db *sqlx.DB, deckID, cardName, tag string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO tamiyo.deck_card_tags (deck_id, card_name, tag) VALUES ($1, $2, $3)`, deckID, cardName, tag)
	require.NoError(t, err)
}

func deckTags(t *testing.T, db *sqlx.DB, deckID string) map[string][]string {
	t.Helper()
	var rows []struct {
		CardName string `db:"card_name"`
		Tag      string `db:"tag"`
	}
	require.NoError(t, db.Select(&rows, `SELECT card_name, tag FROM tamiyo.deck_card_tags WHERE deck_id = $1 ORDER BY card_name, tag`, deckID))
	tags := make(map[string][]string)
	for _, row := range rows {
		tags[row.CardName] = append(tags[row.CardName], row.Tag)
	}
	return tags
}

func TestPostgresRepository_IDsToRefreshAndUpsert(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	const a = "11111111-1111-1111-1111-111111111111"
	const b = "22222222-2222-2222-2222-222222222222"
	const c = "33333333-3333-3333-3333-333333333333"
	seedCard(t, db, alice, a)
	seedCard(t, db, alice, a)
	seedCard(t, db, bob, a)
	seedCard(t, db, bob, b)
	seedCard(t, db, bob, c)

	ids, err := repo.IDsToRefresh(ctx, time.Now().Add(-24*time.Hour), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{a, b, c}, ids)

	require.NoError(t, repo.Upsert(ctx, []Printing{
		{ScryfallID: a, TypeLine: "Creature — Elf", LegalFormats: []string{"commander", "modern"}},
		{ScryfallID: b, TypeLine: "Instant", LegalFormats: []string{}},
	}))
	_, err = db.Exec(`UPDATE tamiyo.printings SET refreshed_at = now() - interval '2 days' WHERE scryfall_id = $1`, b)
	require.NoError(t, err)

	ids, err = repo.IDsToRefresh(ctx, time.Now().Add(-24*time.Hour), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{c, b}, ids)

	ids, err = repo.IDsToRefresh(ctx, time.Now().Add(-24*time.Hour), 1)
	require.NoError(t, err)
	assert.Equal(t, []string{c}, ids)

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Creature — Elf Druid", LegalFormats: []string{"commander"}}}))
	var typeLine string
	var formats []string
	require.NoError(t, db.QueryRow(`SELECT type_line, legal_formats FROM tamiyo.printings WHERE scryfall_id = $1`, a).Scan(&typeLine, pq.Array(&formats)))
	assert.Equal(t, "Creature — Elf Druid", typeLine)
	assert.Equal(t, []string{"commander"}, formats)
}

func TestPostgresRepository_UpsertStoresPrices(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	const a = "11111111-1111-1111-1111-111111111111"
	eur, usdFoil, usdEtched := 1.25, 7.5, 9.75

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Instant", LegalFormats: []string{}, Prices: scryfall.Prices{EUR: &eur, USDFoil: &usdFoil, USDEtched: &usdEtched}}}))

	var gotEUR, gotEURFoil, gotUSD, gotUSDFoil, gotUSDEtched *float64
	require.NoError(t, db.QueryRow(`SELECT eur, eur_foil, usd, usd_foil, usd_etched FROM tamiyo.printings WHERE scryfall_id = $1`, a).
		Scan(&gotEUR, &gotEURFoil, &gotUSD, &gotUSDFoil, &gotUSDEtched))
	require.NotNil(t, gotEUR)
	assert.Equal(t, 1.25, *gotEUR)
	assert.Nil(t, gotEURFoil)
	assert.Nil(t, gotUSD)
	require.NotNil(t, gotUSDFoil)
	assert.Equal(t, 7.5, *gotUSDFoil)
	require.NotNil(t, gotUSDEtched)
	assert.Equal(t, 9.75, *gotUSDEtched)

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Instant", LegalFormats: []string{}}}))
	require.NoError(t, db.QueryRow(`SELECT eur FROM tamiyo.printings WHERE scryfall_id = $1`, a).Scan(&gotEUR))
	assert.Nil(t, gotEUR)
}

func TestPostgresRepository_UpsertStoresPowersAndToughnesses(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	const a = "11111111-1111-1111-1111-111111111111"
	var powers, toughnesses []string

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Creature", LegalFormats: []string{}, Powers: []string{"1", "3"}, Toughnesses: []string{"*"}}}))
	require.NoError(t, db.QueryRow(`SELECT powers, toughnesses FROM tamiyo.printings WHERE scryfall_id = $1`, a).Scan(pq.Array(&powers), pq.Array(&toughnesses)))
	assert.Equal(t, []string{"1", "3"}, powers)
	assert.Equal(t, []string{"*"}, toughnesses)

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Instant", LegalFormats: []string{}}}))
	require.NoError(t, db.QueryRow(`SELECT powers, toughnesses FROM tamiyo.printings WHERE scryfall_id = $1`, a).Scan(pq.Array(&powers), pq.Array(&toughnesses)))
	assert.Empty(t, powers)
	assert.Empty(t, toughnesses)
}

func TestPostgresRepository_UpsertStoresSearchFields(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	const a = "11111111-1111-1111-1111-111111111111"
	var oracleText, manaCost, rarity *string
	var keywords []string

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Creature", LegalFormats: []string{}, ManaCost: "{G}", OracleText: "Trample", Rarity: "rare", Keywords: []string{"Trample"}}}))
	require.NoError(t, db.QueryRow(`SELECT oracle_text, mana_cost, rarity, keywords FROM tamiyo.printings WHERE scryfall_id = $1`, a).Scan(&oracleText, &manaCost, &rarity, pq.Array(&keywords)))
	require.NotNil(t, oracleText)
	require.NotNil(t, manaCost)
	require.NotNil(t, rarity)
	assert.Equal(t, "Trample", *oracleText)
	assert.Equal(t, "{G}", *manaCost)
	assert.Equal(t, "rare", *rarity)
	assert.Equal(t, []string{"Trample"}, keywords)

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Land", LegalFormats: []string{}}}))
	require.NoError(t, db.QueryRow(`SELECT oracle_text, mana_cost, rarity, keywords FROM tamiyo.printings WHERE scryfall_id = $1`, a).Scan(&oracleText, &manaCost, &rarity, pq.Array(&keywords)))
	assert.Nil(t, oracleText)
	assert.Nil(t, manaCost)
	assert.Nil(t, rarity)
	assert.Empty(t, keywords)
}

func TestPostgresRepository_UpsertStoresDeckInsightFields(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	const a = "11111111-1111-1111-1111-111111111111"
	var colors, colorIdentity, banned, restricted, legal []string
	var manaValue float64
	read := func() {
		t.Helper()
		require.NoError(t, db.QueryRow(`
			SELECT colors, color_identity, banned_formats, restricted_formats, legal_formats, mana_value::float8
			FROM tamiyo.printings WHERE scryfall_id = $1
		`, a).Scan(pq.Array(&colors), pq.Array(&colorIdentity), pq.Array(&banned), pq.Array(&restricted), pq.Array(&legal), &manaValue))
	}

	require.NoError(t, repo.Upsert(ctx, []Printing{{
		ScryfallID: a, TypeLine: "Creature", ManaValue: 2.5,
		LegalFormats: []string{"commander", "vintage"}, BannedFormats: []string{"legacy", "modern"}, RestrictedFormats: []string{"vintage"},
		Colors: []string{"G", "U"}, ColorIdentity: []string{"G", "U", "W"},
	}}))
	read()
	assert.Equal(t, []string{"G", "U"}, colors)
	assert.Equal(t, []string{"G", "U", "W"}, colorIdentity)
	assert.Equal(t, []string{"legacy", "modern"}, banned)
	assert.Equal(t, []string{"vintage"}, restricted)
	assert.Equal(t, []string{"commander", "vintage"}, legal)
	assert.Equal(t, 2.5, manaValue)

	require.NoError(t, repo.Upsert(ctx, []Printing{{ScryfallID: a, TypeLine: "Land", LegalFormats: []string{}}}))
	read()
	assert.Empty(t, colors)
	assert.Empty(t, colorIdentity)
	assert.Empty(t, banned)
	assert.Empty(t, restricted)
	assert.Zero(t, manaValue)
}

func TestPostgresRepository_IDsToRefreshIncludesPendingOnlyPrintings(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	alice := seedUser(t, db, "alice@example.com")
	const a = "11111111-1111-1111-1111-111111111111"
	const b = "22222222-2222-2222-2222-222222222222"
	deckID := seedDeck(t, db, alice, seedNamedCard(t, db, alice, a, "Sol Ring"))
	seedPendingCard(t, db, alice, deckID, b, "Arcane Signet")

	ids, err := repo.IDsToRefresh(context.Background(), time.Now().Add(-24*time.Hour), 10)

	require.NoError(t, err)
	assert.Equal(t, []string{a, b}, ids)
}

func TestPostgresRepository_UpsertRenamesCardsAndMovesTheirTags(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")
	const solRing = "11111111-1111-1111-1111-111111111111"
	const otherSolRing = "22222222-2222-2222-2222-222222222222"
	const island = "33333333-3333-3333-3333-333333333333"

	owned := seedDeck(t, db, alice, seedNamedCard(t, db, alice, solRing, "Anneau  solaire"))
	seedTag(t, db, owned, "anneau solaire", "ramp")

	pending := seedDeck(t, db, alice)
	seedPendingCard(t, db, alice, pending, solRing, "Anneau solaire")
	seedTag(t, db, pending, "anneau solaire", "mana")
	seedTag(t, db, pending, "anneau solaire", "ramp")
	seedTag(t, db, pending, "sol ring", "mana")

	shared := seedDeck(t, db, bob, seedNamedCard(t, db, bob, solRing, "Anneau solaire"), seedNamedCard(t, db, bob, otherSolRing, "Anneau solaire"))
	seedTag(t, db, shared, "anneau solaire", "ramp")

	seedNamedCard(t, db, bob, island, "Île")

	require.NoError(t, repo.Upsert(ctx, []Printing{
		{ScryfallID: solRing, Name: "Sol Ring", LegalFormats: []string{}},
		{ScryfallID: island, LegalFormats: []string{}},
	}))

	var names []string
	require.NoError(t, db.Select(&names, `SELECT name FROM tamiyo.cards WHERE scryfall_id = $1`, solRing))
	assert.Equal(t, []string{"Sol Ring", "Sol Ring"}, names)
	require.NoError(t, db.Select(&names, `SELECT name FROM tamiyo.deck_pending_cards WHERE scryfall_id = $1`, solRing))
	assert.Equal(t, []string{"Sol Ring"}, names)
	require.NoError(t, db.Select(&names, `SELECT name FROM tamiyo.cards WHERE scryfall_id = $1`, otherSolRing))
	assert.Equal(t, []string{"Anneau solaire"}, names)
	require.NoError(t, db.Select(&names, `SELECT name FROM tamiyo.cards WHERE scryfall_id = $1`, island))
	assert.Equal(t, []string{"Île"}, names)

	assert.Equal(t, map[string][]string{"sol ring": {"ramp"}}, deckTags(t, db, owned))
	assert.Equal(t, map[string][]string{"sol ring": {"mana", "ramp"}}, deckTags(t, db, pending))
	assert.Equal(t, map[string][]string{"anneau solaire": {"ramp"}, "sol ring": {"ramp"}}, deckTags(t, db, shared), "another card still has the old name")

	var stored string
	require.NoError(t, db.Get(&stored, `SELECT name FROM tamiyo.printings WHERE scryfall_id = $1`, solRing))
	assert.Equal(t, "Sol Ring", stored)
}

func TestPostgresRepository_UpsertSetsTheManaValueOfFoundPrintings(t *testing.T) {
	db := getTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	alice := seedUser(t, db, "alice@example.com")
	const regent = "11111111-1111-1111-1111-111111111111"
	const unknown = "22222222-2222-2222-2222-222222222222"

	seedNamedCard(t, db, alice, regent, "Marang River Regent // Coil and Catch")
	seedPendingCard(t, db, alice, seedDeck(t, db, alice), regent, "Marang River Regent // Coil and Catch")
	_, err := db.Exec(`INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, mana_value) VALUES ($1, 'Lost', $2, 'tst', '1', false, 3)`, alice, unknown)
	require.NoError(t, err)

	require.NoError(t, repo.Upsert(ctx, []Printing{
		{ScryfallID: regent, Name: "Marang River Regent // Coil and Catch", ManaValue: 6, LegalFormats: []string{}},
		{ScryfallID: unknown, LegalFormats: []string{}},
	}))

	var values []float64
	require.NoError(t, db.Select(&values, `SELECT mana_value FROM tamiyo.cards WHERE scryfall_id = $1`, regent))
	assert.Equal(t, []float64{6}, values)
	require.NoError(t, db.Select(&values, `SELECT mana_value FROM tamiyo.deck_pending_cards WHERE scryfall_id = $1`, regent))
	assert.Equal(t, []float64{6}, values)
	require.NoError(t, db.Select(&values, `SELECT mana_value FROM tamiyo.cards WHERE scryfall_id = $1`, unknown))
	assert.Equal(t, []float64{3}, values, "a printing Scryfall no longer knows keeps its mana value")
}
