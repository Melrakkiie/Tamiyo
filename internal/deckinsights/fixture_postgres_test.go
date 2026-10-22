//go:build integration

package deckinsights

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/testdb"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m, &testDB))
}

func getTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	_, err := testDB.Exec(`
		TRUNCATE TABLE tamiyo.deck_pending_cards, tamiyo.card_deck, tamiyo.deck, tamiyo.cards,
			tamiyo.users, tamiyo.printings, tamiyo.exchange_rates
		RESTART IDENTITY CASCADE
	`)
	require.NoError(t, err)
	return testDB
}

// printing is one row of tamiyo.printings, plus the set and collector number
// the deck's copies of it carry. Nil prices are stored as NULL.
type printing struct {
	ScryfallID        string
	Name              string
	SetCode           string
	CollectorNumber   string
	TypeLine          string
	OracleText        string
	Keywords          []string
	ManaValue         float64
	Colors            []string
	ColorIdentity     []string
	LegalFormats      []string
	BannedFormats     []string
	RestrictedFormats []string
	EUR               *float64
	EURFoil           *float64
	USD               *float64
	USDFoil           *float64
	USDEtched         *float64
	GameChanger       bool
}

func seedPrintings(t *testing.T, db *sqlx.DB, printings ...printing) {
	t.Helper()
	for _, p := range printings {
		_, err := db.Exec(`
			INSERT INTO tamiyo.printings (scryfall_id, name, type_line, oracle_text, keywords, mana_value, colors, color_identity,
				legal_formats, banned_formats, restricted_formats, eur, eur_foil, usd, usd_foil, usd_etched, game_changer)
			VALUES ($1, $2, $3, NULLIF($4, ''), COALESCE($5::text[], '{}'), $6, COALESCE($7::text[], '{}'), COALESCE($8::text[], '{}'),
				COALESCE($9::text[], '{}'), COALESCE($10::text[], '{}'), COALESCE($11::text[], '{}'), $12, $13, $14, $15, $16, $17)
		`, p.ScryfallID, p.Name, p.TypeLine, p.OracleText, pq.Array(p.Keywords), p.ManaValue, pq.Array(p.Colors), pq.Array(p.ColorIdentity),
			pq.Array(p.LegalFormats), pq.Array(p.BannedFormats), pq.Array(p.RestrictedFormats),
			p.EUR, p.EURFoil, p.USD, p.USDFoil, p.USDEtched, p.GameChanger)
		require.NoError(t, err)
	}
}

// seedExchangeRate stores how many dollars one euro is worth.
func seedExchangeRate(t *testing.T, db *sqlx.DB, usdPerEUR float64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO tamiyo.exchange_rates (currency, per_eur, rate_date) VALUES ('usd', $1, '2026-10-13')`, usdPerEUR)
	require.NoError(t, err)
}

func seedUser(t *testing.T, db *sqlx.DB, email string) string {
	t.Helper()
	var userID string
	err := db.Get(&userID, `INSERT INTO tamiyo.users (email, password_hash) VALUES ($1, 'fake-hash') RETURNING id`, email)
	require.NoError(t, err)
	return userID
}

type copyOptions struct {
	foil, proxy bool
	board       string // main board when empty
}

// insightsFixture is one user's deck in a real database, filled through
// ownedCopy and pendingCard, read back through the store.
type insightsFixture struct {
	db     *sqlx.DB
	userID string
	deckID string
}

func newInsightsFixture(t *testing.T, format string) insightsFixture {
	t.Helper()
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	var deckID string
	err := db.Get(&deckID, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Deck', $2) RETURNING id`, userID, format)
	require.NoError(t, err)
	return insightsFixture{db: db, userID: userID, deckID: deckID}
}

// otherDeck is another deck of the same user.
func (f insightsFixture) otherDeck(t *testing.T) insightsFixture {
	t.Helper()
	other := f
	err := f.db.Get(&other.deckID, `INSERT INTO tamiyo.deck (user_id, name, format) VALUES ($1, 'Other', 'commander') RETURNING id`, f.userID)
	require.NoError(t, err)
	return other
}

// ownedCopy adds one physical copy of p to the deck.
func (f insightsFixture) ownedCopy(t *testing.T, p printing, opts copyOptions) {
	t.Helper()
	var id int
	err := f.db.Get(&id, `
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, proxy)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, f.userID, p.Name, p.ScryfallID, p.SetCode, p.CollectorNumber, opts.foil, opts.proxy)
	require.NoError(t, err)
	_, err = f.db.Exec(`INSERT INTO tamiyo.card_deck (card_id, deck_id, board) VALUES ($1, $2, $3)`, id, f.deckID, boardOrMain(opts.board))
	require.NoError(t, err)
}

// pendingCard adds quantity copies of p the user doesn't own yet.
func (f insightsFixture) pendingCard(t *testing.T, p printing, quantity int, opts copyOptions) {
	t.Helper()
	_, err := f.db.Exec(`
		INSERT INTO tamiyo.deck_pending_cards (user_id, deck_id, name, scryfall_id, set_code, collector_number, foil, proxy, board, quantity)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, f.userID, f.deckID, p.Name, p.ScryfallID, p.SetCode, p.CollectorNumber, opts.foil, opts.proxy, boardOrMain(opts.board), quantity)
	require.NoError(t, err)
}

func boardOrMain(board string) string {
	if board == "" {
		return deck.BoardMain
	}
	return board
}

func price(v float64) *float64 { return &v }
