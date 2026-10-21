package deckinsights

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/commanderspellbook"
	"Melrakkiie/Tamiyo/internal/deck"
)

// Stored printings for the combo lookups.
var (
	nivMizzet = Printing{ScryfallID: "66666666-0000-0000-0000-000000000001", Name: "Niv-Mizzet, Parun",
		TypeLine: "Legendary Creature — Dragon Wizard", ManaValue: 6}
	thrasiosCombo = Printing{ScryfallID: "66666666-0000-0000-0000-000000000002", Name: "Thrasios, Triton Hero",
		TypeLine: "Legendary Creature — Merfolk Wizard", ManaValue: 2}
	curiosity = Printing{ScryfallID: "66666666-0000-0000-0000-000000000003", Name: "Curiosity",
		TypeLine: "Enchantment — Aura", ManaValue: 1}
	ophidianEye = Printing{ScryfallID: "66666666-0000-0000-0000-000000000004", Name: "Ophidian Eye",
		TypeLine: "Enchantment — Aura", ManaValue: 3}
	delverOfSecrets = Printing{ScryfallID: "66666666-0000-0000-0000-000000000005", Name: "Delver of Secrets // Insectile Aberration",
		TypeLine: "Creature — Human Wizard // Creature — Human Insect", ManaValue: 1}
)

// combosOf looks up the combos of the test user's own deck read, against
// store, through finder.
func combosOf(read deck.ReadDeck, store fakeStore, finder comboFinder) (DeckCombos, error) {
	return NewService(readerWith(read), store, finder).GetDeckCombos(context.Background(), testUserID, read.Deck.ID, deck.ReachOwn)
}

func TestGetDeckCombos_SendsTheCommandersApartFromTheMainBoard(t *testing.T) {
	read := deckOf("commander",
		inSlot(deck.SlotCommander, owned(nivMizzet, 1)),
		inSlot(deck.SlotPartner, pending(thrasiosCombo, 1)),
		owned(curiosity, 2),
		pending(ophidianEye, 1),
	)
	finder := &recordingFinder{}

	_, err := combosOf(read, storeOf(nivMizzet, thrasiosCombo, curiosity, ophidianEye), finder)

	require.NoError(t, err)
	assert.Equal(t, 1, finder.calls)
	assert.Equal(t, []string{"Niv-Mizzet, Parun", "Thrasios, Triton Hero"}, finder.commanders)
	assert.ElementsMatch(t, []string{"Curiosity", "Ophidian Eye"}, finder.main)
}

func TestGetDeckCombos_NamesADoubleFacedCardByBothFaces(t *testing.T) {
	frontFaceOnly := owned(delverOfSecrets, 1)
	frontFaceOnly.Name = "Delver of Secrets"
	finder := &recordingFinder{}

	_, err := combosOf(deckOf("commander", frontFaceOnly), storeOf(delverOfSecrets), finder)

	require.NoError(t, err)
	assert.Equal(t, []string{"Delver of Secrets // Insectile Aberration"}, finder.main)
}

func TestGetDeckCombos_SkipsCardsWithoutAPrinting(t *testing.T) {
	notStored := unstored("Brand New Card", "22222222-0000-0000-0000-00000000000b")
	notFound := unstored("Vanished Card", "22222222-0000-0000-0000-00000000000c")
	read := deckOf("commander", owned(notStored, 1), owned(curiosity, 2), owned(notFound, 3))
	finder := &recordingFinder{}

	_, err := combosOf(read, storeOf(curiosity), finder)

	require.NoError(t, err)
	assert.Equal(t, []string{"Curiosity"}, finder.main)
}

func TestGetDeckCombos_LeavesOutTheSideboardAndConsideringCards(t *testing.T) {
	read := deckOf("commander",
		owned(curiosity, 1),
		onBoard(deck.BoardSideboard, owned(ophidianEye, 2)),
		onBoard(deck.BoardConsidering, pending(nivMizzet, 1)),
	)
	finder := &recordingFinder{}

	_, err := combosOf(read, storeOf(curiosity, ophidianEye, nivMizzet), finder)

	require.NoError(t, err)
	assert.Equal(t, []string{"Curiosity"}, finder.main)
	assert.Empty(t, finder.commanders)
}

func TestGetDeckCombos_SendsEachCardOnce(t *testing.T) {
	foilNiv := pending(nivMizzet, 1)
	foilNiv.Foil = true
	foilCuriosity := owned(curiosity, 3)
	foilCuriosity.Foil = true
	proxyCuriosity := owned(curiosity, 4)
	proxyCuriosity.Proxy = true
	curiosities := owned(curiosity, 2)
	curiosities.Pending = []deck.PendingPart{{ID: 2, Quantity: 2}}
	read := deckOf("commander",
		inSlot(deck.SlotCommander, owned(nivMizzet, 1)),
		curiosities,
		proxyCuriosity,
		foilCuriosity,
		foilNiv,
	)
	finder := &recordingFinder{}

	_, err := combosOf(read, storeOf(nivMizzet, curiosity), finder)

	require.NoError(t, err)
	assert.Equal(t, []string{"Niv-Mizzet, Parun"}, finder.commanders)
	assert.Equal(t, []string{"Curiosity"}, finder.main)
}

func TestGetDeckCombos_AnEmptyMainBoardMakesNoCall(t *testing.T) {
	finder := &recordingFinder{}

	got, err := combosOf(deckOf("commander", onBoard(deck.BoardSideboard, owned(curiosity, 1))), storeOf(curiosity), finder)

	require.NoError(t, err)
	assert.Zero(t, finder.calls)
	assert.NotNil(t, got.Combos)
	assert.Empty(t, got.Combos)
}

func TestGetDeckCombos_ReturnsEachComboWithItsSpellbookPage(t *testing.T) {
	finder := &recordingFinder{combos: []commanderspellbook.Combo{
		{ID: "1414-2730", Cards: []string{"Niv-Mizzet, Parun", "Curiosity"}, Results: []string{"Infinite damage"}},
	}}

	got, err := combosOf(deckOf("commander", owned(curiosity, 1)), storeOf(curiosity), finder)

	require.NoError(t, err)
	assert.Equal(t, []Combo{{
		ID:      "1414-2730",
		Cards:   []string{"Niv-Mizzet, Parun", "Curiosity"},
		Results: []string{"Infinite damage"},
		URL:     "https://commanderspellbook.com/combo/1414-2730/",
	}}, got.Combos)
}

func TestGetDeckCombos_ADeckWithoutCombosReturnsAnEmptyList(t *testing.T) {
	got, err := combosOf(deckOf("commander", owned(curiosity, 1)), storeOf(curiosity), &recordingFinder{})

	require.NoError(t, err)
	assert.NotNil(t, got.Combos)
	assert.Empty(t, got.Combos)
}

func TestGetDeckCombos_AFailedLookupReturnsErrCombosUnavailable(t *testing.T) {
	finder := &recordingFinder{err: errors.New("commander spellbook returned status 503")}

	_, err := combosOf(deckOf("commander", owned(curiosity, 1)), storeOf(curiosity), finder)

	assert.ErrorIs(t, err, ErrCombosUnavailable)
}

func TestGetDeckCombos_UnknownDeckReturnsErrNotFound(t *testing.T) {
	finder := &recordingFinder{}
	svc := NewService(readerWith(deckOf("commander", owned(curiosity, 1))), storeOf(curiosity), finder)

	_, err := svc.GetDeckCombos(context.Background(), testUserID, "00000000-0000-0000-0000-000000000999", deck.ReachOwn)

	assert.ErrorIs(t, err, deck.ErrNotFound)
	assert.Zero(t, finder.calls)
}
