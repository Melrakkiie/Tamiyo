//go:build integration

package deckinsights

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
)

// One printing per case of the price rule, valued at 1.25 dollars per euro.
var (
	regular = printing{ScryfallID: "11111111-0000-0000-0000-000000000001", Name: "Sol Ring", SetCode: "c21", CollectorNumber: "263",
		TypeLine: "Artifact", EUR: price(2), EURFoil: price(5), USD: price(3), USDFoil: price(6)}
	foilOnly = printing{ScryfallID: "11111111-0000-0000-0000-000000000002", Name: "Foil Only", SetCode: "sld", CollectorNumber: "1",
		TypeLine: "Instant", EURFoil: price(4), USDFoil: price(5)}
	etchedOnly = printing{ScryfallID: "11111111-0000-0000-0000-000000000003", Name: "Etched Only", SetCode: "cmr", CollectorNumber: "600",
		TypeLine: "Creature — Elf", USDEtched: price(10)}
	euroMissing = printing{ScryfallID: "11111111-0000-0000-0000-000000000004", Name: "Dollars Only", SetCode: "otj", CollectorNumber: "2",
		TypeLine: "Sorcery", USD: price(2.5)}
	finishMissing = printing{ScryfallID: "11111111-0000-0000-0000-000000000005", Name: "No Foil Price", SetCode: "m21", CollectorNumber: "3",
		TypeLine: "Enchantment", EUR: price(1), USD: price(1.5)}
	noPrice = printing{ScryfallID: "11111111-0000-0000-0000-000000000006", Name: "Priceless", SetCode: "unk", CollectorNumber: "4",
		TypeLine: "Artifact"}
)

func TestGetDeckStats_DeckValuesAgreeWithTheCardListPrices(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, regular, foilOnly, etchedOnly, euroMissing, finishMissing, noPrice)
	seedExchangeRate(t, f.db, 1.25)

	// Main board, owned: 2 + 4 + 8 + 0 euros, 3 + 5 + 10 + 0 dollars.
	commander := f.ownedCopy(t, regular, copyOptions{})
	f.ownedCopy(t, foilOnly, copyOptions{foil: true})
	f.ownedCopy(t, etchedOnly, copyOptions{foil: true})
	f.ownedCopy(t, noPrice, copyOptions{})
	f.ownedCopy(t, regular, copyOptions{foil: true, proxy: true})
	// Main board, pending: 3 × 2 + 2 × 1 euros, 3 × 2.5 + 2 × 1.5 dollars.
	partner := f.pendingCard(t, euroMissing, 3, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)
	f.pendingCard(t, finishMissing, 2, copyOptions{foil: true})
	f.pendingCard(t, regular, 4, copyOptions{proxy: true})
	// Sideboard: 5 + 2 × 2 euros, 6 + 2 × 2.5 dollars.
	f.ownedCopy(t, regular, copyOptions{foil: true, board: deck.BoardSideboard})
	f.pendingCard(t, euroMissing, 2, copyOptions{board: deck.BoardSideboard})
	f.ownedCopy(t, foilOnly, copyOptions{proxy: true, board: deck.BoardSideboard})
	// Considering: 1 euro, 1.5 dollars.
	f.pendingCard(t, finishMissing, 1, copyOptions{board: deck.BoardConsidering})

	stats := f.stats(t)

	assert.Equal(t, Value{EUR: 14, USD: 18}, stats.OwnedValue)
	assert.Equal(t, Value{EUR: 8, USD: 10.5}, stats.PendingValue)
	assert.Equal(t, Value{EUR: 22, USD: 28.5}, stats.Value)
	assert.Equal(t, Value{EUR: 9, USD: 11}, stats.BoardValues.Sideboard)
	assert.Equal(t, Value{EUR: 1, USD: 1.5}, stats.BoardValues.Considering)

	eurList := cardListValues(t, f.cardList(t))
	f.setPriceCurrency(t, "usd")
	usdList := cardListValues(t, f.cardList(t))
	assert.Equal(t, Value{EUR: eurList.owned, USD: usdList.owned}, stats.OwnedValue)
	assert.Equal(t, Value{EUR: eurList.pending, USD: usdList.pending}, stats.PendingValue)
	assert.Equal(t, Value{EUR: eurList.sideboard, USD: usdList.sideboard}, stats.BoardValues.Sideboard)
}

func TestGetDeckStats_DeckWithoutCardsIsWorthNothing(t *testing.T) {
	f := newInsightsFixture(t, "commander")

	stats := f.stats(t)

	assert.Equal(t, Value{}, stats.Value)
	assert.Equal(t, Value{}, stats.OwnedValue)
	assert.Equal(t, Value{}, stats.PendingValue)
	assert.Equal(t, BoardValues{}, stats.BoardValues)
}

type listValues struct{ owned, pending, sideboard float64 }

// cardListValues sums the prices the card list shows next to each non-proxy
// card, in the owner's currency.
func cardListValues(t *testing.T, entries []deck.DeckEntry) listValues {
	t.Helper()
	var v listValues
	for _, e := range entries {
		if e.Proxy || e.Price == nil {
			continue
		}
		switch {
		case e.Board == deck.BoardSideboard:
			v.sideboard += *e.Price * float64(e.Quantity())
		case deck.InMainBoard(e.Board):
			v.owned += *e.Price * float64(e.OwnedQuantity())
			v.pending += *e.Price * float64(e.PendingQuantity())
		}
	}
	require.NotZero(t, v.owned)
	return v
}
