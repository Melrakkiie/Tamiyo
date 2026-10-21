//go:build integration

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

var (
	nivMizzet = printing{ScryfallID: "33333333-0000-0000-0000-000000000001", Name: "Niv-Mizzet, Parun", SetCode: "grn", CollectorNumber: "192",
		TypeLine: "Legendary Creature — Dragon Wizard", ManaValue: 6}
	thrasiosCombo = printing{ScryfallID: "33333333-0000-0000-0000-000000000002", Name: "Thrasios, Triton Hero", SetCode: "c16", CollectorNumber: "46",
		TypeLine: "Legendary Creature — Merfolk Wizard", ManaValue: 2}
	curiosity = printing{ScryfallID: "33333333-0000-0000-0000-000000000003", Name: "Curiosity", SetCode: "a25", CollectorNumber: "45",
		TypeLine: "Enchantment — Aura", ManaValue: 1}
	ophidianEye = printing{ScryfallID: "33333333-0000-0000-0000-000000000004", Name: "Ophidian Eye", SetCode: "tsp", CollectorNumber: "70",
		TypeLine: "Enchantment — Aura", ManaValue: 3}
	delverOfSecrets = printing{ScryfallID: "33333333-0000-0000-0000-000000000005", Name: "Delver of Secrets // Insectile Aberration",
		SetCode: "isd", CollectorNumber: "51", TypeLine: "Creature — Human Wizard // Creature — Human Insect", ManaValue: 1}
)

// recordingFinder stands in for Commander Spellbook: it records the deck it
// is asked about and answers with combos or err.
type recordingFinder struct {
	calls      int
	commanders []string
	main       []string

	combos []commanderspellbook.Combo
	err    error
}

func (r *recordingFinder) FindCombos(ctx context.Context, commanders []string, main []string) ([]commanderspellbook.Combo, error) {
	r.calls++
	r.commanders = commanders
	r.main = main
	return r.combos, r.err
}

func (f insightsFixture) combos(t *testing.T, finder comboFinder) (DeckCombos, error) {
	t.Helper()
	return NewService(f.deckService(), NewPostgresRepository(f.db), finder).GetDeckCombos(context.Background(), f.userID, f.deckID)
}

func TestGetDeckCombos_SendsTheCommandersApartFromTheMainBoard(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, nivMizzet, thrasiosCombo, curiosity, ophidianEye)
	commander := f.ownedCopy(t, nivMizzet, copyOptions{})
	partner := f.pendingCard(t, thrasiosCombo, 1, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)
	f.ownedCopy(t, curiosity, copyOptions{})
	f.pendingCard(t, ophidianEye, 1, copyOptions{})
	finder := &recordingFinder{}

	_, err := f.combos(t, finder)

	require.NoError(t, err)
	assert.Equal(t, 1, finder.calls)
	assert.Equal(t, []string{"Niv-Mizzet, Parun", "Thrasios, Triton Hero"}, finder.commanders)
	assert.ElementsMatch(t, []string{"Curiosity", "Ophidian Eye"}, finder.main)
}

func TestGetDeckCombos_NamesADoubleFacedCardByBothFaces(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, delverOfSecrets)
	frontFaceOnly := delverOfSecrets
	frontFaceOnly.Name = "Delver of Secrets"
	f.ownedCopy(t, frontFaceOnly, copyOptions{})
	finder := &recordingFinder{}

	_, err := f.combos(t, finder)

	require.NoError(t, err)
	assert.Equal(t, []string{"Delver of Secrets // Insectile Aberration"}, finder.main)
}

func TestGetDeckCombos_SkipsCardsWithoutAPrinting(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, curiosity, printing{ScryfallID: notFound.ScryfallID})
	f.ownedCopy(t, curiosity, copyOptions{})
	f.ownedCopy(t, notStored, copyOptions{})
	f.ownedCopy(t, notFound, copyOptions{})
	finder := &recordingFinder{}

	_, err := f.combos(t, finder)

	require.NoError(t, err)
	assert.Equal(t, []string{"Curiosity"}, finder.main)
}

func TestGetDeckCombos_LeavesOutTheSideboardAndConsideringCards(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, curiosity, ophidianEye, nivMizzet)
	f.ownedCopy(t, curiosity, copyOptions{})
	f.ownedCopy(t, ophidianEye, copyOptions{board: deck.BoardSideboard})
	f.pendingCard(t, nivMizzet, 1, copyOptions{board: deck.BoardConsidering})
	finder := &recordingFinder{}

	_, err := f.combos(t, finder)

	require.NoError(t, err)
	assert.Equal(t, []string{"Curiosity"}, finder.main)
	assert.Empty(t, finder.commanders)
}

func TestGetDeckCombos_SendsEachCardOnce(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, nivMizzet, curiosity)
	commander := f.ownedCopy(t, nivMizzet, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, nil)
	f.pendingCard(t, nivMizzet, 1, copyOptions{foil: true})
	f.ownedCopy(t, curiosity, copyOptions{})
	f.ownedCopy(t, curiosity, copyOptions{foil: true})
	f.ownedCopy(t, curiosity, copyOptions{proxy: true})
	f.pendingCard(t, curiosity, 2, copyOptions{})
	finder := &recordingFinder{}

	_, err := f.combos(t, finder)

	require.NoError(t, err)
	assert.Equal(t, []string{"Niv-Mizzet, Parun"}, finder.commanders)
	assert.Equal(t, []string{"Curiosity"}, finder.main)
}

func TestGetDeckCombos_AnEmptyMainBoardMakesNoCall(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, curiosity)
	f.ownedCopy(t, curiosity, copyOptions{board: deck.BoardSideboard})
	finder := &recordingFinder{}

	got, err := f.combos(t, finder)

	require.NoError(t, err)
	assert.Zero(t, finder.calls)
	assert.NotNil(t, got.Combos)
	assert.Empty(t, got.Combos)
}

func TestGetDeckCombos_ReturnsEachComboWithItsSpellbookPage(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, curiosity)
	f.ownedCopy(t, curiosity, copyOptions{})
	finder := &recordingFinder{combos: []commanderspellbook.Combo{
		{ID: "1414-2730", Cards: []string{"Niv-Mizzet, Parun", "Curiosity"}, Results: []string{"Infinite damage"}},
	}}

	got, err := f.combos(t, finder)

	require.NoError(t, err)
	assert.Equal(t, []Combo{{
		ID:      "1414-2730",
		Cards:   []string{"Niv-Mizzet, Parun", "Curiosity"},
		Results: []string{"Infinite damage"},
		URL:     "https://commanderspellbook.com/combo/1414-2730/",
	}}, got.Combos)
}

func TestGetDeckCombos_ADeckWithoutCombosReturnsAnEmptyList(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, curiosity)
	f.ownedCopy(t, curiosity, copyOptions{})

	got, err := f.combos(t, &recordingFinder{})

	require.NoError(t, err)
	assert.NotNil(t, got.Combos)
	assert.Empty(t, got.Combos)
}

func TestGetDeckCombos_AFailedLookupReturnsErrCombosUnavailable(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	seedPrintings(t, f.db, curiosity)
	f.ownedCopy(t, curiosity, copyOptions{})

	_, err := f.combos(t, &recordingFinder{err: errors.New("commander spellbook returned status 503")})

	assert.ErrorIs(t, err, ErrCombosUnavailable)
}

func TestGetDeckCombos_UnknownDeckReturnsErrNotFound(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	finder := &recordingFinder{}
	f.deckID = "00000000-0000-0000-0000-000000000999"

	_, err := f.combos(t, finder)

	assert.ErrorIs(t, err, deck.ErrNotFound)
	assert.Zero(t, finder.calls)
}
