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
	entries := decks.entries[fixtureDeckID]
	tamiyo, island, solRing := entries[0], entries[1], entries[2]
	solRing.Slot = deck.SlotCommander
	tamiyo.Slot = deck.SlotPartner
	decks.entries[fixtureDeckID] = append([]deck.DeckEntry{solRing, tamiyo, island}, entries[3:]...)
	return decks
}

func TestExportDeck_PartnerDecksExportBothCommandersAndNothingElseAsCommander(t *testing.T) {
	decks := partnerDeckFixture()
	extraTamiyo := decks.entries[fixtureDeckID][1]
	extraTamiyo.Slot = deck.SlotNone
	extraTamiyo.Pending = []deck.PendingPart{{ID: 7, Quantity: 2}}
	decks.entries[fixtureDeckID] = append(decks.entries[fixtureDeckID], extraTamiyo)

	out := exportDeckWith(t, decks, DeckExportArena)

	assert.Equal(t, "Commander\n1 Sol Ring\n1 Tamiyo, Inquisitive Student\n\n"+
		"Deck\n5 Island\n2 Tamiyo, Inquisitive Student\n\n"+
		"Sideboard\n1 Duress\n", out)
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
	require.NoError(t, newTestService(&fakeCardService{}, &fakeStorageService{}, partnerDeckFixture(), &fakeResolver{}).ExportTamiyoDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000001", false, &exported))

	file, err := parseTamiyoFile(exported.Bytes())
	require.NoError(t, err)
	assert.True(t, file.deckCards[0].Commander)
	assert.True(t, file.deckCards[1].Commander)
	assert.False(t, file.deckCards[2].Commander)

	cards := &fakeCardService{allCards: []card.Card{{ID: 40, Name: "Sol Ring", ScryfallID: solRingSLD}}}
	decks := withTargetDeck(&fakeDeckService{})
	_, err = newTestService(cards, &fakeStorageService{}, decks, tamiyoResolver()).ImportIntoDeck(context.Background(), testUserID, targetDeckID, DeckImportOptions{}, &exported)

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

func TestBulkEditDeck_ClearsTheSlotOfARemovedPartner(t *testing.T) {
	decks := partnerDeckFixture()
	decks.decks[0].CommanderID = ptr(2)
	decks.decks[0].PartnerPendingID = nil
	decks.decks[0].PartnerID = ptr(1)
	solRing, tamiyo, island := decks.entries[fixtureDeckID][0], decks.entries[fixtureDeckID][1], decks.entries[fixtureDeckID][2]
	solRing.Slot = deck.SlotPartner
	tamiyo.Slot = deck.SlotNone
	islandCommander := island
	islandCommander.Slot = deck.SlotCommander
	islandCommander.OwnedCardIDs = []int{2}
	islandCommander.Pending = nil
	island.OwnedCardIDs = []int{3}
	decks.entries[fixtureDeckID] = append([]deck.DeckEntry{islandCommander, solRing, island, tamiyo}, decks.entries[fixtureDeckID][3:]...)
	svc := newTestService(&fakeCardService{}, &fakeStorageService{}, decks, bulkEditResolver())

	_, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader("5 Island (NEO) 294\n"))

	require.NoError(t, err)
	assert.True(t, decks.commanderCleared)
	assert.Equal(t, deck.SlotPartner, decks.clearedSlot)
}
