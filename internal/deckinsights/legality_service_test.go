package deckinsights

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

// legalPrinting is a stored printing legal in every format of formats.
func legalPrinting(n int, name, typeLine string, colorIdentity []string, formats ...string) Printing {
	return Printing{
		ScryfallID: fmt.Sprintf("33333333-0000-0000-0000-%012d", n), Name: name,
		TypeLine: typeLine, ColorIdentity: colorIdentity, LegalFormats: formats,
	}
}

// storedAs is a card fixture of legality_test.go stored as a commander-legal printing.
func storedAs(n int, c Printing) Printing {
	c.ScryfallID = fmt.Sprintf("44444444-0000-0000-0000-%012d", n)
	c.LegalFormats = []string{"commander"}
	return c
}

func ignoringDeckSize(report LegalityReport) LegalityReport {
	var issues []LegalityIssue
	for _, issue := range report.Issues {
		if !strings.HasPrefix(issue.Reason, deckSizeReasonPrefix) {
			issues = append(issues, issue)
		}
	}
	report.Issues = issues
	report.Legal = len(issues) == 0
	return report
}

func legalityReport(read deck.ReadDeck, store fakeStore) (LegalityReport, error) {
	return NewService(readerWith(read), store, nil).GetDeckLegality(context.Background(), testUserID, read.Deck.ID, deck.ReachOwn)
}

// legalityOf is the legality report of the test user's own deck read,
// against store, without the deck size issue.
func legalityOf(t *testing.T, read deck.ReadDeck, store fakeStore) LegalityReport {
	t.Helper()
	report, err := legalityReport(read, store)
	require.NoError(t, err)
	return ignoringDeckSize(report)
}

func TestGetDeckLegality_ReportsABannedCardFromItsStoredFormatLists(t *testing.T) {
	channel := legalPrinting(1, "Channel", "Sorcery", nil)
	channel.BannedFormats = []string{"commander"}
	ring := legalPrinting(2, "Sol Ring", "Artifact", nil, "commander")

	report := legalityOf(t, deckOf("commander", owned(channel, 7), owned(ring, 8)), storeOf(channel, ring))

	assert.False(t, report.Legal)
	assert.Equal(t, []LegalityIssue{{CardID: 7, CardName: "Channel", Board: deck.BoardMain, Reason: "banned in commander"}}, report.Issues)
}

func TestGetDeckLegality_ReportsARestrictedCard(t *testing.T) {
	// A restricted card is also in the legal list, as the refresher stores it.
	ring := legalPrinting(1, "Sol Ring", "Artifact", nil, "vintage")
	ring.RestrictedFormats = []string{"vintage"}

	report := legalityOf(t, deckOf("vintage", owned(ring, 7)), storeOf(ring))

	assert.Equal(t, []LegalityIssue{{CardID: 7, CardName: "Sol Ring", Board: deck.BoardMain, Reason: "restricted in vintage"}}, report.Issues)
}

func TestGetDeckLegality_ReportsACardInNoFormatListAsNotLegal(t *testing.T) {
	legal := legalPrinting(1, "Lightning Bolt", "Instant", []string{"R"}, "modern")
	notModern := legalPrinting(2, "Shahrazad", "Sorcery", []string{"W"}, "vintage")

	report := legalityOf(t, deckOf("Modern", owned(legal, 7), owned(notModern, 8)), storeOf(legal, notModern))

	assert.Equal(t, []LegalityIssue{{CardID: 8, CardName: "Shahrazad", Board: deck.BoardMain, Reason: "not legal in Modern"}}, report.Issues)
}

func TestGetDeckLegality_ChecksEveryBoard(t *testing.T) {
	bad := legalPrinting(1, "Shahrazad", "Sorcery", nil)
	read := deckOf("modern",
		onBoard(deck.BoardSideboard, owned(bad, 7)),
		onBoard(deck.BoardConsidering, pending(bad, 1)),
	)

	report := legalityOf(t, read, storeOf(bad))

	require.Len(t, report.Issues, 2)
	assert.ElementsMatch(t, []string{deck.BoardSideboard, deck.BoardConsidering}, []string{report.Issues[0].Board, report.Issues[1].Board})
}

func TestGetDeckLegality_AcceptsALegalDeck(t *testing.T) {
	ring := legalPrinting(1, "Sol Ring", "Artifact", nil, "commander")

	report := legalityOf(t, deckOf("commander", owned(ring, 7)), storeOf(ring))

	assert.True(t, report.Legal)
	assert.Empty(t, report.Issues)
	assert.Equal(t, "commander", report.Format)
}

func TestGetDeckLegality_ReportsACardWithNoStoredPrintingAsUnverified(t *testing.T) {
	// notStored isn't stored yet, notFound is a placeholder the store leaves out.
	notStored := unstored("Brand New Card", "22222222-0000-0000-0000-00000000000b")
	notFound := unstored("Vanished Card", "22222222-0000-0000-0000-00000000000c")

	report := legalityOf(t, deckOf("commander", owned(notStored, 7), pending(notFound, 1)), storeOf())

	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 2)
	for _, issue := range report.Issues {
		assert.Equal(t, "could not verify legality: not found on scryfall", issue.Reason)
	}
}

func TestGetDeckLegality_RejectsAFormatScryfallDoesNotKnow(t *testing.T) {
	ring := legalPrinting(1, "Sol Ring", "Artifact", nil, "commander")

	_, err := legalityReport(deckOf("not-a-real-format", owned(ring, 7)), storeOf(ring))

	assert.ErrorIs(t, err, ErrUnknownFormat)
}

func TestGetDeckLegality_ReportsASingletonViolationButNotDuplicateBasicLands(t *testing.T) {
	ring := legalPrinting(1, "Sol Ring", "Artifact", nil, "commander")
	forest := legalPrinting(2, "Forest", "Basic Land — Forest", []string{"G"}, "commander")
	forests := owned(forest, 7)
	forests.Pending = []deck.PendingPart{{ID: 1, Quantity: 30}}
	foilRing := owned(ring, 9)
	foilRing.Foil = true
	read := deckOf("commander", forests, owned(ring, 8), foilRing)

	report := legalityOf(t, read, storeOf(ring, forest))

	assert.Equal(t, []LegalityIssue{{
		CardName: "Sol Ring", Reason: "singleton violation: 2 copies in deck (commander allows only 1, except basic lands)",
	}}, report.Issues)
}

func TestGetDeckLegality_ReportsACardOutsideTheCommandersColorIdentity(t *testing.T) {
	golosCard := legalPrinting(1, "Golos, Tireless Pilgrim", "Legendary Artifact Creature — Scout", nil, "commander")
	krenko := legalPrinting(2, "Krenko, Mob Boss", "Legendary Creature — Goblin", []string{"R"}, "commander")
	bolt := legalPrinting(3, "Lightning Bolt", "Instant", []string{"R"}, "commander")
	read := deckOf("commander", inSlot(deck.SlotCommander, owned(golosCard, 7)), owned(bolt, 8))

	report := legalityOf(t, read, storeOf(golosCard, krenko, bolt))

	assert.Equal(t, []LegalityIssue{{
		CardID: 8, CardName: "Lightning Bolt", Board: deck.BoardMain,
		Reason: "outside commander's color identity (card: R, commander: )",
	}}, report.Issues)
}

func TestGetDeckLegality_AcceptsACardWithinTheCommandersColorIdentity(t *testing.T) {
	krenko := legalPrinting(1, "Krenko, Mob Boss", "Legendary Creature — Goblin", []string{"R"}, "commander")
	bolt := legalPrinting(2, "Lightning Bolt", "Instant", []string{"R"}, "commander")
	read := deckOf("commander", inSlot(deck.SlotCommander, owned(krenko, 7)), owned(bolt, 8))

	assert.True(t, legalityOf(t, read, storeOf(krenko, bolt)).Legal)
}

func TestGetDeckLegality_AcceptsAValidPartnerPairAndUsesBothIdentities(t *testing.T) {
	thr, tym := storedAs(1, thrasios), storedAs(2, tymna)
	bolt := legalPrinting(3, "Lightning Bolt", "Instant", []string{"R"}, "commander")
	swords := legalPrinting(4, "Swords to Plowshares", "Instant", []string{"W"}, "commander")
	read := deckOf("commander",
		inSlot(deck.SlotCommander, owned(thr, 7)),
		inSlot(deck.SlotPartner, pending(tym, 1)),
		owned(bolt, 8),
		owned(swords, 9),
	)

	report := legalityOf(t, read, storeOf(thr, tym, bolt, swords))

	require.Len(t, report.Issues, 1)
	assert.Equal(t, "Lightning Bolt", report.Issues[0].CardName)
	assert.Contains(t, report.Issues[0].Reason, "commander: WUBG")
}

func TestGetDeckLegality_ReportsCommandersThatCantBePaired(t *testing.T) {
	thr, gol := storedAs(1, thrasios), storedAs(2, golos)
	read := deckOf("commander", inSlot(deck.SlotCommander, owned(thr, 7)), inSlot(deck.SlotPartner, owned(gol, 8)))

	report := legalityOf(t, read, storeOf(thr, gol))

	assert.Equal(t, []LegalityIssue{{
		CardID: 8, CardName: "Golos, Tireless Pilgrim", Board: deck.BoardMain, Reason: "these two commanders can't be paired",
	}}, report.Issues)
}

func TestGetDeckLegality_ChecksTheDeckSize(t *testing.T) {
	forest := legalPrinting(1, "Forest", "Basic Land — Forest", []string{"G"}, "commander")

	report, err := legalityReport(deckOf("commander", pending(forest, 99)), storeOf(forest))

	require.NoError(t, err)
	assert.False(t, report.Legal)
	assert.Equal(t, []LegalityIssue{{Reason: "deck size: 99 cards, commander requires exactly 100"}}, report.Issues)
}

func TestDeckSizeRulesOnlyCoverFormatsScryfallRecognizes(t *testing.T) {
	for format := range deckSizeRules {
		assert.True(t, card.IsLegalFormat(format), format)
	}
}
