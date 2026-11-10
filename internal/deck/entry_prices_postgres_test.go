//go:build integration

package deck

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeckContent_CarriesThePricePerCopyInBothCurrencies(t *testing.T) {
	f := newContentFixture(t)
	_, err := f.db.Exec(`INSERT INTO tamiyo.exchange_rates (currency, per_eur, rate_date) VALUES ('usd', 2, now())`)
	require.NoError(t, err)
	_, err = f.db.Exec(`INSERT INTO tamiyo.printings (scryfall_id, type_line, eur, eur_foil, usd) VALUES ($1, 'Instant', 0.50, 2.00, 0.60)`, bolt.scryfallID)
	require.NoError(t, err)
	_, err = f.db.Exec(`INSERT INTO tamiyo.printings (scryfall_id, type_line, usd) VALUES ($1, 'Land', 3.00)`, island.scryfallID)
	require.NoError(t, err)
	f.ownedCopy(t, bolt, copyOptions{})
	f.ownedCopy(t, bolt, copyOptions{foil: true})
	f.ownedCopy(t, island, copyOptions{})
	f.ownedCopy(t, tymna, copyOptions{})

	type prices struct {
		name     string
		foil     bool
		eur, usd *float64
	}
	var got []prices
	for _, e := range f.content(t).Entries {
		got = append(got, prices{e.Name, e.Foil, e.PriceEUR, e.PriceUSD})
	}

	f64 := func(v float64) *float64 { return &v }
	require.Equal(t, []prices{
		{island.name, false, f64(1.5), f64(3)}, // dollar price only: euro price converted
		{bolt.name, false, f64(0.5), f64(0.6)},
		{bolt.name, true, f64(2), f64(0.6)}, // no foil dollar price: falls back to the other finish
		{tymna.name, false, nil, nil},       // unknown printing
	}, got)
}
