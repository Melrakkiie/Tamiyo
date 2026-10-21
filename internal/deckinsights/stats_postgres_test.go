//go:build integration

package deckinsights

import (
	"context"
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

// Card printings for the statistics, read from tamiyo.printings.
var (
	mountain = printing{ScryfallID: "22222222-0000-0000-0000-000000000001", Name: "Mountain", SetCode: "fdn", CollectorNumber: "279",
		TypeLine: "Basic Land — Mountain"}
	solRing = printing{ScryfallID: "22222222-0000-0000-0000-000000000002", Name: "Sol Ring", SetCode: "c21", CollectorNumber: "263",
		TypeLine: "Artifact", ManaValue: 1}
	shock = printing{ScryfallID: "22222222-0000-0000-0000-000000000003", Name: "Shock", SetCode: "m19", CollectorNumber: "156",
		TypeLine: "Instant", ManaValue: 1, Colors: []string{"R"}}
	mindStone = printing{ScryfallID: "22222222-0000-0000-0000-000000000004", Name: "Mind Stone", SetCode: "wth", CollectorNumber: "153",
		TypeLine: "Artifact", ManaValue: 2}
	arcaneSignet = printing{ScryfallID: "22222222-0000-0000-0000-000000000005", Name: "Arcane Signet", SetCode: "eld", CollectorNumber: "331",
		TypeLine: "Artifact", ManaValue: 2}
	balaGed = printing{ScryfallID: "22222222-0000-0000-0000-000000000006", Name: "Bala Ged Recovery // Bala Ged Sanctuary", SetCode: "znr",
		CollectorNumber: "180", TypeLine: "Sorcery // Land", ManaValue: 3, Colors: []string{"G"}}
	atraxa = printing{ScryfallID: "22222222-0000-0000-0000-000000000007", Name: "Atraxa, Praetors' Voice", SetCode: "c16", CollectorNumber: "28",
		TypeLine: "Legendary Creature — Phyrexian Angel Horror", ManaValue: 4, Colors: []string{"G", "W", "U", "B"}}
	construct = printing{ScryfallID: "22222222-0000-0000-0000-000000000008", Name: "Steel Overseer", SetCode: "m11", CollectorNumber: "214",
		TypeLine: "Artifact Creature — Construct", ManaValue: 3}
	golosPrinting = printing{ScryfallID: "22222222-0000-0000-0000-000000000009", Name: "Golos, Tireless Pilgrim", SetCode: "m20", CollectorNumber: "226",
		TypeLine: "Legendary Artifact Creature — Scout", ManaValue: 5}
	wastes = printing{ScryfallID: "22222222-0000-0000-0000-00000000000a", Name: "Wastes", SetCode: "ogw", CollectorNumber: "183",
		TypeLine: "Basic Land"}
	// notStored has no printing row yet; notFound is the placeholder the
	// refresher stores for a printing Scryfall didn't return.
	notStored = printing{ScryfallID: "22222222-0000-0000-0000-00000000000b", Name: "Brand New Card", SetCode: "new", CollectorNumber: "1"}
	notFound  = printing{ScryfallID: "22222222-0000-0000-0000-00000000000c", Name: "Vanished Card", SetCode: "old", CollectorNumber: "2"}
	// littleGirl has a half mana value.
	littleGirl = printing{ScryfallID: "22222222-0000-0000-0000-00000000000d", Name: "Little Girl", SetCode: "unh", CollectorNumber: "9",
		TypeLine: "Creature — Human Child", ManaValue: 0.5, Colors: []string{"W"}}
)

func TestGetDeckStats_LandsOnlyDeckHasAnEmptyManaCurve(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, mountain)
	f.ownedCopy(t, mountain, copyOptions{})

	stats := f.stats(t)

	require.NotNil(t, stats.ManaCurve)
	assert.Empty(t, stats.ManaCurve)
	require.NotNil(t, stats.BackFaceLands)
	assert.Empty(t, stats.BackFaceLands)
	assert.Zero(t, stats.AverageManaValue)
}

func TestGetDeckStats_SplitsTheManaCurveIntoPermanentsAndListsItsCards(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, shock, mindStone, arcaneSignet, balaGed, mountain)
	f.ownedCopy(t, shock, copyOptions{})
	f.ownedCopy(t, shock, copyOptions{})
	f.ownedCopy(t, mindStone, copyOptions{})
	f.ownedCopy(t, balaGed, copyOptions{})
	f.ownedCopy(t, balaGed, copyOptions{foil: true})
	f.ownedCopy(t, arcaneSignet, copyOptions{})
	f.ownedCopy(t, mountain, copyOptions{})

	stats := f.stats(t)

	assert.InDelta(t, 12.0, stats.TotalManaValue, 1e-9) // 2 Shock + Mind Stone + 2 Bala Ged (front face, 3) + Arcane Signet
	assert.Equal(t, []ManaCurveBucket{
		{ManaValue: 1, Count: 2, NonPermanents: 2, Cards: []ManaCurveCard{
			{Name: "Shock", ScryfallID: shock.ScryfallID, Quantity: 2, Type: "Instant"},
		}},
		{ManaValue: 2, Count: 2, Permanents: 2, Cards: []ManaCurveCard{
			{Name: "Arcane Signet", ScryfallID: arcaneSignet.ScryfallID, Quantity: 1, Type: "Artifact"},
			{Name: "Mind Stone", ScryfallID: mindStone.ScryfallID, Quantity: 1, Type: "Artifact"},
		}},
		{ManaValue: 3, Count: 2, NonPermanents: 2, Cards: []ManaCurveCard{
			{Name: balaGed.Name, ScryfallID: balaGed.ScryfallID, Quantity: 2, Type: "Sorcery"},
		}},
	}, stats.ManaCurve)
	assert.Equal(t, []BackFaceCard{{Name: balaGed.Name, ScryfallID: balaGed.ScryfallID, Quantity: 2}}, stats.BackFaceLands)
	assert.Equal(t, 1, stats.LandCount)
	assert.Equal(t, 6, stats.NonlandCount)
	assert.InDelta(t, 2.0, stats.AverageManaValue, 1e-9)
	assert.Equal(t, map[string]int{"R": 2, "G": 2, "C": 2}, stats.ColorBreakdown)
}

func TestGetDeckStats_MulticolorCardCountsUnderEachColor(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, atraxa)
	f.ownedCopy(t, atraxa, copyOptions{})

	stats := f.stats(t)

	assert.Equal(t, map[string]int{"W": 1, "U": 1, "B": 1, "G": 1}, stats.ColorBreakdown)
}

func TestGetDeckStats_ArtifactCreatureCountsAsCreature(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, construct)
	f.ownedCopy(t, construct, copyOptions{})

	stats := f.stats(t)

	assert.Equal(t, map[string]int{"Creature": 1}, stats.TypeBreakdown)
}

func TestGetDeckStats_ACardWithoutAStoredPrintingCountsAsUnknown(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, solRing, printing{ScryfallID: notFound.ScryfallID})
	f.ownedCopy(t, solRing, copyOptions{})
	f.ownedCopy(t, notStored, copyOptions{})
	f.pendingCard(t, notFound, 2, copyOptions{})

	stats := f.stats(t)

	assert.Equal(t, 4, stats.CardCount)
	assert.Equal(t, map[string]int{"Artifact": 1, "Unknown": 3}, stats.TypeBreakdown)
	assert.Zero(t, stats.LandCount)
	assert.Equal(t, 1, stats.NonlandCount)
	assert.InDelta(t, 1.0, stats.TotalManaValue, 1e-9)
	assert.Equal(t, map[string]int{"C": 1}, stats.ColorBreakdown)
	require.Len(t, stats.ManaCurve, 1)
	assert.Equal(t, 1, stats.ManaCurve[0].Count)
}

func TestGetDeckStats_UnknownDeckReturnsErrDeckNotFound(t *testing.T) {
	f := newInsightsFixture(t, "commander")

	_, err := f.service().GetDeckStats(context.Background(), f.userID, "00000000-0000-0000-0000-000000000999")

	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestGetDeckStats_CountsPendingCardsWithTheirQuantities(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, mountain, shock)
	f.ownedCopy(t, mountain, copyOptions{})
	f.pendingCard(t, mountain, 3, copyOptions{})
	f.pendingCard(t, shock, 2, copyOptions{foil: true})

	stats := f.stats(t)

	assert.Equal(t, 6, stats.CardCount)
	assert.Equal(t, 4, stats.LandCount)
	assert.Equal(t, 2, stats.NonlandCount)
	assert.InDelta(t, 2.0, stats.TotalManaValue, 1e-9)
	assert.Equal(t, map[string]int{"R": 2}, stats.ColorBreakdown)
	assert.Equal(t, []ManaCurveBucket{
		{ManaValue: 1, Count: 2, NonPermanents: 2, Cards: []ManaCurveCard{{Name: "Shock", ScryfallID: shock.ScryfallID, Quantity: 2, Type: "Instant"}}},
	}, stats.ManaCurve)
}

func TestGetDeckStats_CountsAPendingCardOf1500Copies(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, golosPrinting, wastes, shock)
	commander := f.pendingCard(t, golosPrinting, 1, copyOptions{})
	f.setCommanders(t, nil, &commander, nil, nil)
	f.pendingCard(t, wastes, 1500, copyOptions{})
	f.pendingCard(t, shock, 1500, copyOptions{})

	stats := f.stats(t)

	assert.Equal(t, 3001, stats.CardCount)
	assert.Equal(t, 1500, stats.LandCount)
	assert.Equal(t, 1501, stats.NonlandCount)
	assert.Equal(t, map[string]int{"R": 1500, "C": 1}, stats.ColorBreakdown)
	assert.Equal(t, map[string]int{"Land": 1500, "Instant": 1500, "Creature": 1}, stats.TypeBreakdown)
	assert.Equal(t, []ManaCurveBucket{
		{ManaValue: 1, Count: 1500, NonPermanents: 1500, Cards: []ManaCurveCard{{Name: "Shock", ScryfallID: shock.ScryfallID, Quantity: 1500, Type: "Instant"}}},
		{ManaValue: 5, Count: 1, Permanents: 1, Cards: []ManaCurveCard{{Name: golosPrinting.Name, ScryfallID: golosPrinting.ScryfallID, Quantity: 1, Type: "Creature"}}},
	}, stats.ManaCurve)
}

func TestGetDeckStats_OnlyCountsTheMainBoard(t *testing.T) {
	f := newInsightsFixture(t, "modern")
	seedPrintings(t, f.db, mountain, shock, mindStone)
	f.ownedCopy(t, mountain, copyOptions{})
	f.ownedCopy(t, mindStone, copyOptions{})
	f.ownedCopy(t, shock, copyOptions{board: deck.BoardSideboard})
	f.pendingCard(t, shock, 3, copyOptions{board: deck.BoardConsidering})

	stats := f.stats(t)

	assert.InDelta(t, 2.0, stats.TotalManaValue, 1e-9)
	assert.Equal(t, 2, stats.CardCount)
	assert.Equal(t, map[string]int{"Land": 1, "Artifact": 1}, stats.TypeBreakdown)
}

func TestGetDeckStats_TotalManaValueSumsHalfManaValuesPerCopy(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, littleGirl, shock, mountain)
	f.pendingCard(t, littleGirl, 3, copyOptions{})
	f.ownedCopy(t, shock, copyOptions{})
	f.ownedCopy(t, mountain, copyOptions{})

	stats := f.stats(t)

	assert.InDelta(t, 2.5, stats.TotalManaValue, 1e-9)
	assert.Equal(t, 4, stats.NonlandCount)
}

func TestGetDeckStats_DeckWithoutNonlandsHasNoTotalManaValue(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, mountain)
	f.ownedCopy(t, mountain, copyOptions{})

	assert.Zero(t, f.stats(t).TotalManaValue)
}

func TestGetDeckStats_SeparatesLandsFromNonlands(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, mountain, solRing)
	f.ownedCopy(t, mountain, copyOptions{})
	f.ownedCopy(t, solRing, copyOptions{})

	stats := f.stats(t)

	assert.Equal(t, 2, stats.CardCount)
	assert.Equal(t, 1, stats.LandCount)
	assert.Equal(t, 1, stats.NonlandCount)
	assert.Equal(t, 1.0, stats.AverageManaValue)
	assert.Equal(t, map[string]int{"C": 1}, stats.ColorBreakdown)
	assert.Equal(t, map[string]int{"Land": 1, "Artifact": 1}, stats.TypeBreakdown)
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
