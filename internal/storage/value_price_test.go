//go:build integration

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepository_StorageValue_EqualsTheSumOfTheCardPrices(t *testing.T) {
	db := getTestDB(t)
	userID := seedUser(t, db, "alice@example.com")
	repo := NewPostgresRepository(db)
	seedStorages(t, db, userID)
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
	_, err = db.Exec(`
		INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id) VALUES
			($1, 'Regular', $2, 'tst', '1', false, false, 1),
			($1, 'Foil only', $3, 'tst', '2', true, false, 1),
			($1, 'Etched only', $4, 'tst', '3', true, false, 1),
			($1, 'Euro missing', $5, 'tst', '4', false, false, 1),
			($1, 'Finish missing', $6, 'tst', '5', true, false, 1),
			($1, 'Unpriced', $7, 'tst', '6', false, false, 1),
			($1, 'Proxy', $2, 'tst', '1', false, true, 1)
	`, userID, regular, foilOnly, etchedOnly, euroMissing, finishMissing, unpriced)
	require.NoError(t, err)

	found, err := repo.FindByID(context.Background(), userID, 1)

	require.NoError(t, err)
	assert.Equal(t, 7, found.CardCount)
	// euros: 1 + 3.25 + 10/1.25 + 2.50/1.25 + 5; dollars: 1.50 + 4 + 10 + 2.50 + 6
	assert.InDelta(t, 19.25, found.Value.EUR, 0.001)
	assert.InDelta(t, 24, found.Value.USD, 0.001)

	all, _, err := repo.FindAll(context.Background(), userID, defaultFilter())
	require.NoError(t, err)
	for _, s := range all {
		if s.ID == 1 {
			assert.InDelta(t, 19.25, s.Value.EUR, 0.001)
			assert.InDelta(t, 24, s.Value.USD, 0.001)
		}
	}
}
