//go:build integration

package card

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepository_Value_EqualsTheSumOfTheCardPrices(t *testing.T) {
	db := getTestDB(t)
	alice := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	const regular = "11111111-1111-1111-1111-111111111111"
	const foilOnly = "22222222-2222-2222-2222-222222222222"
	const etchedOnly = "33333333-3333-3333-3333-333333333333"
	const euroMissing = "44444444-4444-4444-4444-444444444444"
	const finishMissing = "55555555-5555-5555-5555-555555555555"
	const unpriced = "66666666-6666-6666-6666-666666666666"
	_, err := db.Exec(`
		INSERT INTO tamiyo.printings (scryfall_id, type_line, eur, eur_foil, usd, usd_foil, usd_etched) VALUES
			($1, 'Instant', 1.00, NULL, 1.50, NULL, NULL),
			($2, 'Instant', NULL, 3.25, NULL, 4.00, NULL),
			($3, 'Instant', NULL, NULL, NULL, NULL, 10.00),
			($4, 'Instant', NULL, NULL, 2.50, NULL, NULL),
			($5, 'Instant', 5.00, NULL, 6.00, NULL, NULL),
			($6, 'Instant', NULL, NULL, NULL, NULL, NULL)
	`, regular, foilOnly, etchedOnly, euroMissing, finishMissing, unpriced)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tamiyo.exchange_rates (currency, per_eur, rate_date) VALUES ('usd', 1.25, '2026-10-13')`)
	require.NoError(t, err)
	seedCards(t, db, alice, []Card{
		{Name: "Regular", ScryfallID: regular, SetCode: "tst", CollectorNumber: "1"},
		{Name: "Foil only", ScryfallID: foilOnly, SetCode: "tst", CollectorNumber: "2", Foil: true},
		{Name: "Etched only", ScryfallID: etchedOnly, SetCode: "tst", CollectorNumber: "3", Foil: true},
		{Name: "Euro missing", ScryfallID: euroMissing, SetCode: "tst", CollectorNumber: "4"},
		{Name: "Finish missing", ScryfallID: finishMissing, SetCode: "tst", CollectorNumber: "5", Foil: true},
		{Name: "Unpriced", ScryfallID: unpriced, SetCode: "tst", CollectorNumber: "6"},
		{Name: "Proxy", ScryfallID: regular, SetCode: "tst", CollectorNumber: "1", Proxy: true},
	})

	value, err := repo.Value(context.Background(), alice)

	require.NoError(t, err)
	// euros: 1 + 3.25 + 10/1.25 + 2.50/1.25 + 5; dollars: 1.50 + 4 + 10 + 2.50 + 6
	assert.InDelta(t, 19.25, value.EUR, 0.001)
	assert.InDelta(t, 24, value.USD, 0.001)

	var want struct {
		EUR float64 `db:"eur"`
		USD float64 `db:"usd"`
	}
	require.NoError(t, db.Get(&want, `
		SELECT COALESCE(SUM(COALESCE(tamiyo.card_price(scryfall_id, foil, 'eur'), 0)), 0)::float8 AS eur,
			COALESCE(SUM(COALESCE(tamiyo.card_price(scryfall_id, foil, 'usd'), 0)), 0)::float8 AS usd
		FROM tamiyo.cards
		WHERE user_id = $1 AND NOT proxy
	`, alice))
	assert.InDelta(t, want.EUR, value.EUR, 0.001)
	assert.InDelta(t, want.USD, value.USD, 0.001)
}
