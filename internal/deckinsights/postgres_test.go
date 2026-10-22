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

// notStored has no printing row yet; notFound is the placeholder the
// refresher stores for a printing Scryfall didn't return.
var (
	notStored = printing{ScryfallID: "22222222-0000-0000-0000-00000000000b", Name: "Brand New Card", SetCode: "new", CollectorNumber: "1"}
	notFound  = printing{ScryfallID: "22222222-0000-0000-0000-00000000000c", Name: "Vanished Card", SetCode: "old", CollectorNumber: "2"}
)

func (f insightsFixture) deckValues(t *testing.T) DeckValues {
	t.Helper()
	values, err := NewPostgresRepository(f.db).DeckValues(context.Background(), f.userID, f.deckID)
	require.NoError(t, err)
	return values
}

func assertValue(t *testing.T, want Value, got Value) {
	t.Helper()
	assert.InDelta(t, want.EUR, got.EUR, 1e-9, "EUR")
	assert.InDelta(t, want.USD, got.USD, 1e-9, "USD")
}

func TestPostgresRepository_DeckValues_SplitsOwnedPendingSideboardAndConsidering(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, regular, foilOnly, etchedOnly, euroMissing, finishMissing, noPrice)
	seedExchangeRate(t, f.db, 1.25)

	// Main board, owned: 2 + 4 + 8 + 0 euros, 3 + 5 + 10 + 0 dollars.
	f.ownedCopy(t, regular, copyOptions{})
	f.ownedCopy(t, foilOnly, copyOptions{foil: true})
	f.ownedCopy(t, etchedOnly, copyOptions{foil: true})
	f.ownedCopy(t, noPrice, copyOptions{})
	f.ownedCopy(t, regular, copyOptions{foil: true, proxy: true})
	// Main board, pending: 3 × 2 + 2 × 1 euros, 3 × 2.5 + 2 × 1.5 dollars.
	f.pendingCard(t, euroMissing, 3, copyOptions{})
	f.pendingCard(t, finishMissing, 2, copyOptions{foil: true})
	f.pendingCard(t, regular, 4, copyOptions{proxy: true})
	// Sideboard: 5 + 2 × 2 euros, 6 + 2 × 2.5 dollars.
	f.ownedCopy(t, regular, copyOptions{foil: true, board: deck.BoardSideboard})
	f.pendingCard(t, euroMissing, 2, copyOptions{board: deck.BoardSideboard})
	f.ownedCopy(t, foilOnly, copyOptions{proxy: true, board: deck.BoardSideboard})
	// Considering: 1 euro, 1.5 dollars.
	f.pendingCard(t, finishMissing, 1, copyOptions{board: deck.BoardConsidering})

	values := f.deckValues(t)

	assertValue(t, Value{EUR: 14, USD: 18}, values.Owned)
	assertValue(t, Value{EUR: 8, USD: 10.5}, values.Pending)
	assertValue(t, Value{EUR: 9, USD: 11}, values.Sideboard)
	assertValue(t, Value{EUR: 1, USD: 1.5}, values.Considering)
}

func TestPostgresRepository_DeckValues_DeckWithoutCardsIsWorthNothing(t *testing.T) {
	f := newInsightsFixture(t, "commander")

	assert.Equal(t, DeckValues{}, f.deckValues(t))
}

func TestPostgresRepository_DeckValues_LeavesOutAnotherDeck(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, regular)
	seedExchangeRate(t, f.db, 1.25)
	f.ownedCopy(t, regular, copyOptions{})
	other := f.otherDeck(t)
	other.ownedCopy(t, regular, copyOptions{foil: true})
	other.pendingCard(t, regular, 2, copyOptions{})

	values := f.deckValues(t)

	assert.Equal(t, DeckValues{Owned: Value{EUR: 2, USD: 3}}, values)
}

func TestPostgresRepository_Printings_ReadsEachStoredPrintingByScryfallID(t *testing.T) {
	db := getTestDB(t)
	ring := printing{
		ScryfallID: "55555555-0000-0000-0000-000000000001", Name: "Sol Ring", TypeLine: "Artifact", OracleText: "{T}: Add {C}{C}.",
		ManaValue: 1, ColorIdentity: []string{}, LegalFormats: []string{"commander", "vintage"}, RestrictedFormats: []string{"vintage"},
		BannedFormats: []string{"legacy"},
	}
	bolt := printing{
		ScryfallID: "55555555-0000-0000-0000-000000000002", Name: "Lightning Bolt", TypeLine: "Instant",
		Keywords: []string{"Flash"}, ManaValue: 1, Colors: []string{"R"}, ColorIdentity: []string{"R"}, LegalFormats: []string{"modern"},
	}
	unread := printing{ScryfallID: "55555555-0000-0000-0000-000000000003", Name: "Unread", TypeLine: "Sorcery"}
	seedPrintings(t, db, ring, bolt, unread)

	got, err := NewPostgresRepository(db).Printings(context.Background(), []string{ring.ScryfallID, bolt.ScryfallID})

	require.NoError(t, err)
	assert.Equal(t, map[string]Printing{
		ring.ScryfallID: {
			ScryfallID: ring.ScryfallID, Name: "Sol Ring", TypeLine: "Artifact", OracleText: "{T}: Add {C}{C}.",
			Keywords: []string{}, ManaValue: 1, Colors: []string{}, ColorIdentity: []string{},
			LegalFormats: []string{"commander", "vintage"}, BannedFormats: []string{"legacy"}, RestrictedFormats: []string{"vintage"},
		},
		bolt.ScryfallID: {
			ScryfallID: bolt.ScryfallID, Name: "Lightning Bolt", TypeLine: "Instant",
			Keywords: []string{"Flash"}, ManaValue: 1, Colors: []string{"R"}, ColorIdentity: []string{"R"},
			LegalFormats: []string{"modern"}, BannedFormats: []string{}, RestrictedFormats: []string{},
		},
	}, got)
}

func TestPostgresRepository_Printings_LeavesOutUnstoredPrintingsAndPlaceholders(t *testing.T) {
	db := getTestDB(t)
	stored := printing{ScryfallID: "55555555-0000-0000-0000-000000000001", Name: "Sol Ring", TypeLine: "Artifact"}
	seedPrintings(t, db, stored, printing{ScryfallID: notFound.ScryfallID})

	got, err := NewPostgresRepository(db).Printings(context.Background(),
		[]string{stored.ScryfallID, notStored.ScryfallID, notFound.ScryfallID})

	require.NoError(t, err)
	assert.Equal(t, []string{stored.ScryfallID}, keysOf(got))
}

func TestPostgresRepository_Printings_NoScryfallIDsReadsNothing(t *testing.T) {
	got, err := NewPostgresRepository(getTestDB(t)).Printings(context.Background(), nil)

	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func keysOf(printings map[string]Printing) []string {
	keys := make([]string, 0, len(printings))
	for id := range printings {
		keys = append(keys, id)
	}
	return keys
}
