package bulk

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

func partnerDeckFixture() *fakeDeckService {
	decks := tamiyoDeckFixture()
	decks.decks[0].CommanderPendingID = nil
	decks.decks[0].CommanderID = ptr(1)
	decks.decks[0].PartnerPendingID = ptr(7)
	return decks
}

func TestExportDeck_ArenaListsBothCommanders(t *testing.T) {
	out := exportDeckWith(t, partnerDeckFixture(), DeckExportArena)

	assert.True(t, strings.HasPrefix(out, "Commander\n1 Sol Ring\n1 Tamiyo, Inquisitive Student\n\nDeck\n"), out)
}

func TestExportDeck_MoxfieldPutsBothCommandersFirst(t *testing.T) {
	out := exportDeckWith(t, partnerDeckFixture(), DeckExportMoxfield)

	lines := strings.Split(out, "\n")
	assert.Equal(t, []string{"1 Sol Ring (sld) 1011", "1 Tamiyo, Inquisitive Student (neo) 75"}, lines[:2])
}

func TestTamiyoDeck_RoundTripsTwoCommanders(t *testing.T) {
	var exported bytes.Buffer
	require.NoError(t, NewService(&fakeCardService{}, &fakeStorageService{}, partnerDeckFixture(), &fakeResolver{}).ExportTamiyoDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", false, &exported))

	file, err := parseTamiyoFile(exported.Bytes())
	require.NoError(t, err)
	assert.True(t, file.deckCards[0].Commander)
	assert.True(t, file.deckCards[1].Commander)
	assert.False(t, file.deckCards[2].Commander)

	cards := &fakeCardService{allCards: []card.Card{{ID: 40, Name: "Sol Ring", ScryfallID: solRingSLD}}}
	decks := withTargetDeck(&fakeDeckService{})
	_, err = NewService(cards, &fakeStorageService{}, decks, tamiyoResolver()).ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, &exported)

	require.NoError(t, err)
	assert.Equal(t, 40, decks.cardCommanderID)
	require.NotEmpty(t, decks.addedPending)
	assert.Equal(t, "Tamiyo, Inquisitive Student", decks.addedPending[0].Name)
	assert.Equal(t, decks.addedPending[0].ID, decks.pendingPartnerID)
	assert.Zero(t, decks.pendingCommanderID)
}

func TestImportIntoDeck_TakesUpToTwoCommandersFromTheCommanderSection(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 5, Name: "Atraxa, Praetors' Voice", ScryfallID: atraxaID}}}
	decks := &fakeDeckService{}

	importDeck(t, cards, decks, "Commander\n1 Atraxa, Praetors' Voice (CMR) 1\n1 Sol Ring (SLD) 1011\n\nDeck\n1 Sol Ring (SLD) 1011\n", false)

	assert.Equal(t, 5, decks.cardCommanderID)
	require.Len(t, decks.addedPending, 2)
	assert.Equal(t, decks.addedPending[0].ID, decks.pendingPartnerID)
	assert.Zero(t, decks.pendingCommanderID)
}

func TestImportIntoDeck_FirstLineStillMarksOneCommander(t *testing.T) {
	cards := &fakeCardService{allCards: []card.Card{{ID: 5, Name: "Atraxa, Praetors' Voice", ScryfallID: atraxaID}}}
	decks := &fakeDeckService{}

	importDeck(t, cards, decks, "1 Atraxa, Praetors' Voice (CMR) 1\n1 Sol Ring (SLD) 1011\n", true)

	assert.Equal(t, 5, decks.cardCommanderID)
	assert.Zero(t, decks.pendingPartnerID)
	assert.Zero(t, decks.cardPartnerID)
}

func TestParseMoxfieldDeckList_MarksAtMostTwoCommanders(t *testing.T) {
	lines, err := parseMoxfieldDeckList(strings.NewReader("Commander\n1 Thrasios\n1 Tymna\n1 Kraum\nDeck\n1 Sol Ring\n"))

	require.NoError(t, err)
	var commanders []string
	for _, line := range lines {
		if line.Commander {
			commanders = append(commanders, line.CardName)
		}
		assert.Equal(t, deck.BoardMain, line.Board)
	}
	assert.Equal(t, []string{"Thrasios", "Tymna"}, commanders)
}

func TestSwitchFoil_KeepsThePartnerSlot(t *testing.T) {
	cards, decks := foilFixture()
	decks.decks[0].CommanderPendingID = ptr(99)
	decks.decks[0].PartnerID = ptr(1)
	cards.allCards = append(cards.allCards, card.Card{ID: 30, Name: "Sol Ring", ScryfallID: solRingSLD, Foil: true})
	svc := NewService(cards, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.SwitchFoil(context.Background(), testUserID, fixtureDeckID, FoilSwitch{CardIDs: []int{1}, Foil: true})

	require.NoError(t, err)
	assert.Equal(t, 30, decks.cardPartnerID)
	assert.Zero(t, decks.cardCommanderID)
}

func TestBulkEditDeck_ClearsTheSlotOfARemovedPartner(t *testing.T) {
	decks := partnerDeckFixture()
	decks.decks[0].CommanderID = ptr(2)
	decks.decks[0].PartnerPendingID = nil
	decks.decks[0].PartnerID = ptr(1)
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, bulkEditResolver())

	_, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader("5 Island (NEO) 294\n"))

	require.NoError(t, err)
	assert.True(t, decks.commanderCleared)
	assert.Equal(t, deck.SlotPartner, decks.clearedSlot)
}
