//go:build integration

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
func legalPrinting(n int, name, typeLine string, colorIdentity []string, formats ...string) printing {
	return printing{
		ScryfallID: fmt.Sprintf("33333333-0000-0000-0000-%012d", n), Name: name, SetCode: "tst", CollectorNumber: fmt.Sprint(n),
		TypeLine: typeLine, ColorIdentity: colorIdentity, LegalFormats: formats,
	}
}

// storedAs seeds a card fixture of legality_test.go as a commander-legal printing.
func storedAs(n int, c Printing) printing {
	return printing{
		ScryfallID: fmt.Sprintf("44444444-0000-0000-0000-%012d", n), Name: c.Name, SetCode: "tst", CollectorNumber: fmt.Sprint(n),
		TypeLine: c.TypeLine, OracleText: c.OracleText, ColorIdentity: c.ColorIdentity, LegalFormats: []string{"commander"},
	}
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

// legality is the deck's legality report without the deck size issue.
func (f insightsFixture) legality(t *testing.T) LegalityReport {
	t.Helper()
	report, err := f.service().GetDeckLegality(context.Background(), f.userID, f.deckID)
	require.NoError(t, err)
	return ignoringDeckSize(report)
}

func TestGetDeckLegality_ReportsABannedCardFromItsStoredFormatLists(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	channel := legalPrinting(1, "Channel", "Sorcery", nil)
	channel.BannedFormats = []string{"commander"}
	ring := legalPrinting(2, "Sol Ring", "Artifact", nil, "commander")
	seedPrintings(t, f.db, channel, ring)
	id := f.ownedCopy(t, channel, copyOptions{})
	f.ownedCopy(t, ring, copyOptions{})

	report := f.legality(t)

	assert.False(t, report.Legal)
	assert.Equal(t, []LegalityIssue{{CardID: id, CardName: "Channel", Board: deck.BoardMain, Reason: "banned in commander"}}, report.Issues)
}

func TestGetDeckLegality_ReportsARestrictedCard(t *testing.T) {
	f := newInsightsFixture(t, "vintage")
	// A restricted card is also in the legal list, as the refresher stores it.
	ring := legalPrinting(1, "Sol Ring", "Artifact", nil, "vintage")
	ring.RestrictedFormats = []string{"vintage"}
	seedPrintings(t, f.db, ring)
	id := f.ownedCopy(t, ring, copyOptions{})

	report := f.legality(t)

	assert.Equal(t, []LegalityIssue{{CardID: id, CardName: "Sol Ring", Board: deck.BoardMain, Reason: "restricted in vintage"}}, report.Issues)
}

func TestGetDeckLegality_ReportsACardInNoFormatListAsNotLegal(t *testing.T) {
	f := newInsightsFixture(t, "Modern")
	legal := legalPrinting(1, "Lightning Bolt", "Instant", []string{"R"}, "modern")
	notModern := legalPrinting(2, "Shahrazad", "Sorcery", []string{"W"}, "vintage")
	seedPrintings(t, f.db, legal, notModern)
	f.ownedCopy(t, legal, copyOptions{})
	id := f.ownedCopy(t, notModern, copyOptions{})

	report := f.legality(t)

	assert.Equal(t, []LegalityIssue{{CardID: id, CardName: "Shahrazad", Board: deck.BoardMain, Reason: "not legal in Modern"}}, report.Issues)
}

func TestGetDeckLegality_ChecksEveryBoard(t *testing.T) {
	f := newInsightsFixture(t, "modern")
	bad := legalPrinting(1, "Shahrazad", "Sorcery", nil)
	seedPrintings(t, f.db, bad)
	f.ownedCopy(t, bad, copyOptions{board: deck.BoardSideboard})
	f.pendingCard(t, bad, 1, copyOptions{board: deck.BoardConsidering})

	report := f.legality(t)

	require.Len(t, report.Issues, 2)
	assert.ElementsMatch(t, []string{deck.BoardSideboard, deck.BoardConsidering}, []string{report.Issues[0].Board, report.Issues[1].Board})
}

func TestGetDeckLegality_AcceptsALegalDeck(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	ring := legalPrinting(1, "Sol Ring", "Artifact", nil, "commander")
	seedPrintings(t, f.db, ring)
	f.ownedCopy(t, ring, copyOptions{})

	report := f.legality(t)

	assert.True(t, report.Legal)
	assert.Empty(t, report.Issues)
	assert.Equal(t, "commander", report.Format)
}

func TestGetDeckLegality_ReportsACardWithNoStoredPrintingAsUnverified(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	// notStored has no row, notFound is the refresher's placeholder.
	seedPrintings(t, f.db, notFound)
	f.ownedCopy(t, notStored, copyOptions{})
	f.pendingCard(t, notFound, 1, copyOptions{})

	report := f.legality(t)

	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 2)
	for _, issue := range report.Issues {
		assert.Equal(t, "could not verify legality: not found on scryfall", issue.Reason)
	}
}

func TestGetDeckLegality_RejectsAFormatScryfallDoesNotKnow(t *testing.T) {
	f := newInsightsFixture(t, "not-a-real-format")
	ring := legalPrinting(1, "Sol Ring", "Artifact", nil, "commander")
	seedPrintings(t, f.db, ring)
	f.ownedCopy(t, ring, copyOptions{})

	_, err := f.service().GetDeckLegality(context.Background(), f.userID, f.deckID)

	assert.ErrorIs(t, err, ErrUnknownFormat)
}

func TestGetDeckLegality_ReportsASingletonViolationButNotDuplicateBasicLands(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	ring := legalPrinting(1, "Sol Ring", "Artifact", nil, "commander")
	forest := legalPrinting(2, "Forest", "Basic Land — Forest", []string{"G"}, "commander")
	seedPrintings(t, f.db, ring, forest)
	f.ownedCopy(t, ring, copyOptions{})
	f.ownedCopy(t, ring, copyOptions{foil: true})
	f.ownedCopy(t, forest, copyOptions{})
	f.pendingCard(t, forest, 30, copyOptions{})

	report := f.legality(t)

	assert.Equal(t, []LegalityIssue{{
		CardName: "Sol Ring", Reason: "singleton violation: 2 copies in deck (commander allows only 1, except basic lands)",
	}}, report.Issues)
}

func TestGetDeckLegality_ReportsACardOutsideTheCommandersColorIdentity(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	golosCard := legalPrinting(1, "Golos, Tireless Pilgrim", "Legendary Artifact Creature — Scout", nil, "commander")
	krenko := legalPrinting(2, "Krenko, Mob Boss", "Legendary Creature — Goblin", []string{"R"}, "commander")
	bolt := legalPrinting(3, "Lightning Bolt", "Instant", []string{"R"}, "commander")
	seedPrintings(t, f.db, golosCard, krenko, bolt)
	commander := f.ownedCopy(t, golosCard, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, nil)
	boltID := f.ownedCopy(t, bolt, copyOptions{})

	report := f.legality(t)

	assert.Equal(t, []LegalityIssue{{
		CardID: boltID, CardName: "Lightning Bolt", Board: deck.BoardMain,
		Reason: "outside commander's color identity (card: R, commander: )",
	}}, report.Issues)
}

func TestGetDeckLegality_AcceptsACardWithinTheCommandersColorIdentity(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	krenko := legalPrinting(1, "Krenko, Mob Boss", "Legendary Creature — Goblin", []string{"R"}, "commander")
	bolt := legalPrinting(2, "Lightning Bolt", "Instant", []string{"R"}, "commander")
	seedPrintings(t, f.db, krenko, bolt)
	commander := f.ownedCopy(t, krenko, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, nil)
	f.ownedCopy(t, bolt, copyOptions{})

	assert.True(t, f.legality(t).Legal)
}

func TestGetDeckLegality_AcceptsAValidPartnerPairAndUsesBothIdentities(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	thr, tym := storedAs(1, thrasios), storedAs(2, tymna)
	bolt := legalPrinting(3, "Lightning Bolt", "Instant", []string{"R"}, "commander")
	swords := legalPrinting(4, "Swords to Plowshares", "Instant", []string{"W"}, "commander")
	seedPrintings(t, f.db, thr, tym, bolt, swords)
	commander := f.ownedCopy(t, thr, copyOptions{})
	partner := f.pendingCard(t, tym, 1, copyOptions{})
	f.setCommanders(t, &commander, nil, nil, &partner)
	f.ownedCopy(t, bolt, copyOptions{})
	f.ownedCopy(t, swords, copyOptions{})

	report := f.legality(t)

	require.Len(t, report.Issues, 1)
	assert.Equal(t, "Lightning Bolt", report.Issues[0].CardName)
	assert.Contains(t, report.Issues[0].Reason, "commander: WUBG")
}

func TestGetDeckLegality_ReportsCommandersThatCantBePaired(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	thr, gol := storedAs(1, thrasios), storedAs(2, golos)
	seedPrintings(t, f.db, thr, gol)
	commander := f.ownedCopy(t, thr, copyOptions{})
	partner := f.ownedCopy(t, gol, copyOptions{})
	f.setCommanders(t, &commander, nil, &partner, nil)

	report := f.legality(t)

	assert.Equal(t, []LegalityIssue{{
		CardID: partner, CardName: "Golos, Tireless Pilgrim", Board: deck.BoardMain, Reason: "these two commanders can't be paired",
	}}, report.Issues)
}

func TestGetDeckLegality_ChecksTheDeckSize(t *testing.T) {
	f := newInsightsFixture(t, "commander")
	forest := legalPrinting(1, "Forest", "Basic Land — Forest", []string{"G"}, "commander")
	seedPrintings(t, f.db, forest)
	f.pendingCard(t, forest, 99, copyOptions{})

	report, err := f.service().GetDeckLegality(context.Background(), f.userID, f.deckID)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	assert.Equal(t, []LegalityIssue{{Reason: "deck size: 99 cards, commander requires exactly 100"}}, report.Issues)
}

func TestDeckSizeRulesOnlyCoverFormatsScryfallRecognizes(t *testing.T) {
	for format := range deckSizeRules {
		assert.True(t, card.IsLegalFormat(format), format)
	}
}
