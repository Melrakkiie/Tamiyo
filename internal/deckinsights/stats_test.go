package deckinsights

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
)

// Stored printings for the statistics.
var (
	mountain = Printing{ScryfallID: "22222222-0000-0000-0000-000000000001", Name: "Mountain", TypeLine: "Basic Land — Mountain"}
	solRing  = Printing{ScryfallID: "22222222-0000-0000-0000-000000000002", Name: "Sol Ring", TypeLine: "Artifact", ManaValue: 1}
	shock    = Printing{ScryfallID: "22222222-0000-0000-0000-000000000003", Name: "Shock", TypeLine: "Instant", ManaValue: 1,
		Colors: []string{"R"}}
	mindStone    = Printing{ScryfallID: "22222222-0000-0000-0000-000000000004", Name: "Mind Stone", TypeLine: "Artifact", ManaValue: 2}
	arcaneSignet = Printing{ScryfallID: "22222222-0000-0000-0000-000000000005", Name: "Arcane Signet", TypeLine: "Artifact", ManaValue: 2}
	balaGed      = Printing{ScryfallID: "22222222-0000-0000-0000-000000000006", Name: "Bala Ged Recovery // Bala Ged Sanctuary",
		TypeLine: "Sorcery // Land", ManaValue: 3, Colors: []string{"G"}}
	atraxa = Printing{ScryfallID: "22222222-0000-0000-0000-000000000007", Name: "Atraxa, Praetors' Voice",
		TypeLine: "Legendary Creature — Phyrexian Angel Horror", ManaValue: 4, Colors: []string{"G", "W", "U", "B"}}
	construct = Printing{ScryfallID: "22222222-0000-0000-0000-000000000008", Name: "Steel Overseer",
		TypeLine: "Artifact Creature — Construct", ManaValue: 3}
	golosPrinting = Printing{ScryfallID: "22222222-0000-0000-0000-000000000009", Name: "Golos, Tireless Pilgrim",
		TypeLine: "Legendary Artifact Creature — Scout", ManaValue: 5}
	wastes = Printing{ScryfallID: "22222222-0000-0000-0000-00000000000a", Name: "Wastes", TypeLine: "Basic Land"}
	// littleGirl has a half mana value.
	littleGirl = Printing{ScryfallID: "22222222-0000-0000-0000-00000000000d", Name: "Little Girl",
		TypeLine: "Creature — Human Child", ManaValue: 0.5, Colors: []string{"W"}}
	// Game changers: rhysticStudy and rhysticStudyRetro are two printings of
	// one card.
	kinnan = Printing{ScryfallID: "22222222-0000-0000-0000-00000000000e", Name: "Kinnan, Bonder Prodigy",
		TypeLine: "Legendary Creature — Human Druid", ManaValue: 2, Colors: []string{"G", "U"}}
	rhysticStudy = Printing{ScryfallID: "22222222-0000-0000-0000-00000000000f", Name: "Rhystic Study",
		TypeLine: "Enchantment", ManaValue: 3, Colors: []string{"U"}}
	rhysticStudyRetro = Printing{ScryfallID: "22222222-0000-0000-0000-000000000010", Name: "Rhystic Study",
		TypeLine: "Enchantment", ManaValue: 3, Colors: []string{"U"}}
	cyclonicRift = Printing{ScryfallID: "22222222-0000-0000-0000-000000000011", Name: "Cyclonic Rift",
		TypeLine: "Instant", ManaValue: 2, Colors: []string{"U"}}
	gaeasCradle = Printing{ScryfallID: "22222222-0000-0000-0000-000000000012", Name: "Gaea's Cradle",
		TypeLine: "Legendary Land"}
	demonicTutor = Printing{ScryfallID: "22222222-0000-0000-0000-000000000013", Name: "Demonic Tutor",
		TypeLine: "Sorcery", ManaValue: 2, Colors: []string{"B"}}
	vampiricTutor = Printing{ScryfallID: "22222222-0000-0000-0000-000000000014", Name: "Vampiric Tutor",
		TypeLine: "Instant", ManaValue: 1, Colors: []string{"B"}}
)

// statsOf is the statistics of the test user's own deck read, against store.
func statsOf(t *testing.T, read deck.ReadDeck, store fakeStore) DeckStats {
	t.Helper()
	stats, err := NewService(readerWith(read), store, nil).GetDeckStats(context.Background(), testUserID, read.Deck.ID, deck.ReachOwn)
	require.NoError(t, err)
	return stats
}

func TestGetDeckStats_ReportsTheDeckValuesRoundedToCents(t *testing.T) {
	store := storeOf(solRing)
	store.values = DeckValues{
		Owned:       Value{EUR: 14.004, USD: 18.006},
		Pending:     Value{EUR: 8, USD: 10.5},
		Sideboard:   Value{EUR: 9.001, USD: 11},
		Considering: Value{EUR: 1, USD: 1.499},
	}

	stats := statsOf(t, deckOf("commander", owned(solRing, 1)), store)

	assert.Equal(t, Value{EUR: 14, USD: 18.01}, stats.OwnedValue)
	assert.Equal(t, Value{EUR: 8, USD: 10.5}, stats.PendingValue)
	assert.Equal(t, Value{EUR: 22, USD: 28.51}, stats.Value)
	assert.Equal(t, Value{EUR: 9, USD: 11}, stats.BoardValues.Sideboard)
	assert.Equal(t, Value{EUR: 1, USD: 1.5}, stats.BoardValues.Considering)
}

func TestGetDeckStats_DeckWithoutCardsIsWorthNothing(t *testing.T) {
	stats := statsOf(t, deckOf("commander"), storeOf())

	assert.Equal(t, Value{}, stats.Value)
	assert.Equal(t, Value{}, stats.OwnedValue)
	assert.Equal(t, Value{}, stats.PendingValue)
	assert.Equal(t, BoardValues{}, stats.BoardValues)
}

func TestGetDeckStats_LandsOnlyDeckHasAnEmptyManaCurve(t *testing.T) {
	stats := statsOf(t, deckOf("commander", owned(mountain, 1)), storeOf(mountain))

	require.NotNil(t, stats.ManaCurve)
	assert.Empty(t, stats.ManaCurve)
	require.NotNil(t, stats.BackFaceLands)
	assert.Empty(t, stats.BackFaceLands)
	assert.Zero(t, stats.AverageManaValue)
}

func TestGetDeckStats_SplitsTheManaCurveIntoPermanentsAndListsItsCards(t *testing.T) {
	foilBalaGed := owned(balaGed, 5)
	foilBalaGed.Foil = true
	read := deckOf("commander",
		owned(arcaneSignet, 6),
		owned(balaGed, 4),
		foilBalaGed,
		owned(mindStone, 3),
		owned(mountain, 7),
		owned(shock, 1, 2),
	)

	stats := statsOf(t, read, storeOf(shock, mindStone, arcaneSignet, balaGed, mountain))

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
	stats := statsOf(t, deckOf("commander", owned(atraxa, 1)), storeOf(atraxa))

	assert.Equal(t, map[string]int{"W": 1, "U": 1, "B": 1, "G": 1}, stats.ColorBreakdown)
}

func TestGetDeckStats_ArtifactCreatureCountsAsCreature(t *testing.T) {
	stats := statsOf(t, deckOf("commander", owned(construct, 1)), storeOf(construct))

	assert.Equal(t, map[string]int{"Creature": 1}, stats.TypeBreakdown)
}

func TestGetDeckStats_ACardWithoutAStoredPrintingCountsAsUnknown(t *testing.T) {
	notStored := unstored("Brand New Card", "22222222-0000-0000-0000-00000000000b")
	notFound := unstored("Vanished Card", "22222222-0000-0000-0000-00000000000c")
	read := deckOf("commander", owned(notStored, 2), owned(solRing, 1), pending(notFound, 2))

	stats := statsOf(t, read, storeOf(solRing))

	assert.Equal(t, 4, stats.CardCount)
	assert.Equal(t, map[string]int{"Artifact": 1, "Unknown": 3}, stats.TypeBreakdown)
	assert.Zero(t, stats.LandCount)
	assert.Equal(t, 1, stats.NonlandCount)
	assert.InDelta(t, 1.0, stats.TotalManaValue, 1e-9)
	assert.Equal(t, map[string]int{"C": 1}, stats.ColorBreakdown)
	require.Len(t, stats.ManaCurve, 1)
	assert.Equal(t, 1, stats.ManaCurve[0].Count)
}

func TestGetDeckStats_CardCountOverKnownPrintingsFollowsTheDecksCardCountRule(t *testing.T) {
	entries := []deck.DeckEntry{
		inSlot(deck.SlotCommander, owned(kinnan, 1)),
		inSlot(deck.SlotPartner, pending(rhysticStudy, 1)),
		owned(solRing, 2, 3),
		pending(mountain, 3),
		onBoard(deck.BoardSideboard, owned(solRing, 4, 5, 6, 7, 8)),
		onBoard(deck.BoardConsidering, pending(mountain, 4)),
	}

	stats := statsOf(t, deckOf("commander", entries...), storeOf(kinnan, rhysticStudy, solRing, mountain))

	deckCardCount := 0
	for _, e := range entries {
		if deck.InMainBoard(e.Board) {
			deckCardCount += e.Quantity()
		}
	}
	assert.Equal(t, 7, deckCardCount)
	assert.Equal(t, deckCardCount, stats.CardCount)
	assert.NotContains(t, stats.TypeBreakdown, "Unknown")
}

func TestGetDeckStats_UnknownDeckReturnsErrNotFound(t *testing.T) {
	svc := NewService(readerWith(deckOf("commander")), storeOf(), nil)

	_, err := svc.GetDeckStats(context.Background(), testUserID, "00000000-0000-0000-0000-000000000999", deck.ReachOwn)

	assert.ErrorIs(t, err, deck.ErrNotFound)
}

func TestGetDeckStats_CountsPendingCardsWithTheirQuantities(t *testing.T) {
	foilShock := pending(shock, 2)
	foilShock.Foil = true
	mountains := owned(mountain, 1)
	mountains.Pending = []deck.PendingPart{{ID: 1, Quantity: 3}}
	read := deckOf("commander", mountains, foilShock)

	stats := statsOf(t, read, storeOf(mountain, shock))

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
	read := deckOf("commander",
		inSlot(deck.SlotCommander, pending(golosPrinting, 1)),
		pending(shock, 1500),
		pending(wastes, 1500),
	)

	stats := statsOf(t, read, storeOf(golosPrinting, wastes, shock))

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
	read := deckOf("modern",
		owned(mindStone, 2),
		owned(mountain, 1),
		onBoard(deck.BoardSideboard, owned(shock, 3)),
		onBoard(deck.BoardConsidering, pending(shock, 3)),
	)

	stats := statsOf(t, read, storeOf(mountain, shock, mindStone))

	assert.InDelta(t, 2.0, stats.TotalManaValue, 1e-9)
	assert.Equal(t, 2, stats.CardCount)
	assert.Equal(t, map[string]int{"Land": 1, "Artifact": 1}, stats.TypeBreakdown)
}

func TestGetDeckStats_CountsEachGameChangerOfTheMainBoardOnce(t *testing.T) {
	foilRhystic := gameChanger(owned(rhysticStudy, 3))
	foilRhystic.Foil = true
	read := deckOf("commander",
		inSlot(deck.SlotCommander, gameChanger(owned(kinnan, 1))),
		gameChanger(owned(rhysticStudy, 1, 2)),
		foilRhystic,
		gameChanger(pending(rhysticStudyRetro, 2)),
		gameChanger(pending(cyclonicRift, 1)),
		gameChanger(owned(gaeasCradle, 4)),
		owned(solRing, 5),
	)

	stats := statsOf(t, read, storeOf(kinnan, rhysticStudy, rhysticStudyRetro, cyclonicRift, gaeasCradle, solRing))

	assert.Equal(t, 4, stats.GameChangerCount)
}

func TestGetDeckStats_GameChangerCountIgnoresSideBoardsAndUnstoredCards(t *testing.T) {
	read := deckOf("commander",
		owned(solRing, 1),
		gameChanger(owned(unstored("Mana Crypt", "22222222-0000-0000-0000-000000000015"), 2)),
		onBoard(deck.BoardSideboard, gameChanger(owned(demonicTutor, 3))),
		onBoard(deck.BoardConsidering, gameChanger(pending(vampiricTutor, 1))),
	)

	stats := statsOf(t, read, storeOf(solRing, demonicTutor, vampiricTutor))

	assert.Zero(t, stats.GameChangerCount)
}

func TestGetDeckStats_TotalManaValueSumsHalfManaValuesPerCopy(t *testing.T) {
	read := deckOf("commander", pending(littleGirl, 3), owned(mountain, 2), owned(shock, 1))

	stats := statsOf(t, read, storeOf(littleGirl, shock, mountain))

	assert.InDelta(t, 2.5, stats.TotalManaValue, 1e-9)
	assert.Equal(t, 4, stats.NonlandCount)
}

func TestGetDeckStats_DeckWithoutNonlandsHasNoTotalManaValue(t *testing.T) {
	assert.Zero(t, statsOf(t, deckOf("commander", owned(mountain, 1)), storeOf(mountain)).TotalManaValue)
}

func TestGetDeckStats_SeparatesLandsFromNonlands(t *testing.T) {
	stats := statsOf(t, deckOf("commander", owned(mountain, 1), owned(solRing, 2)), storeOf(mountain, solRing))

	assert.Equal(t, 2, stats.CardCount)
	assert.Equal(t, 1, stats.LandCount)
	assert.Equal(t, 1, stats.NonlandCount)
	assert.Equal(t, 1.0, stats.AverageManaValue)
	assert.Equal(t, map[string]int{"C": 1}, stats.ColorBreakdown)
	assert.Equal(t, map[string]int{"Land": 1, "Artifact": 1}, stats.TypeBreakdown)
}
