package deckinsights

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

const testDeckID = "00000000-0000-0000-0000-000000000001"

// --- fakes -------------------------------------------------------------

type fakeDeckService struct {
	contents map[string]deck.DeckContent
	err      error
}

func (f *fakeDeckService) GetDeckContent(ctx context.Context, userID string, deckID string) (deck.DeckContent, error) {
	if f.err != nil {
		return deck.DeckContent{}, f.err
	}
	content, ok := f.contents[deckID]
	if !ok {
		return deck.DeckContent{}, deck.ErrNotFound
	}
	return content, nil
}

func (f *fakeDeckService) add(entries ...deck.DeckEntry) {
	content := f.contents[testDeckID]
	content.Entries = append(content.Entries, entries...)
	f.contents[testDeckID] = content
}

func deckWith(d deck.Deck, entries ...deck.DeckEntry) *fakeDeckService {
	d.ID = testDeckID
	return &fakeDeckService{contents: map[string]deck.DeckContent{testDeckID: {Deck: d, Entries: entries}}}
}

func owned(name, scryfallID string, cardIDs ...int) deck.DeckEntry {
	return deck.DeckEntry{Name: name, ScryfallID: scryfallID, Board: deck.BoardMain, OwnedCardIDs: cardIDs}
}

func pending(name, scryfallID string, pendingID, quantity int) deck.DeckEntry {
	return deck.DeckEntry{Name: name, ScryfallID: scryfallID, Board: deck.BoardMain, Pending: []deck.PendingPart{{ID: pendingID, Quantity: quantity}}}
}

func onBoard(board string, e deck.DeckEntry) deck.DeckEntry {
	e.Board = board
	return e
}

func inSlot(slot deck.CommanderSlot, e deck.DeckEntry) deck.DeckEntry {
	e.Slot = slot
	return e
}

func foil(e deck.DeckEntry) deck.DeckEntry {
	e.Foil = true
	return e
}

func withPending(pendingID, quantity int, e deck.DeckEntry) deck.DeckEntry {
	e.Pending = append(e.Pending, deck.PendingPart{ID: pendingID, Quantity: quantity})
	return e
}

type fakeScryfallFetcher struct {
	cards       map[string]scryfall.Card // keyed by Scryfall ID
	err         error
	lastRequest []scryfall.Identifier
}

func (f *fakeScryfallFetcher) Fetch(ctx context.Context, identifiers []scryfall.Identifier) ([]scryfall.Card, error) {
	f.lastRequest = identifiers
	if f.err != nil {
		return nil, f.err
	}
	var out []scryfall.Card
	for _, id := range identifiers {
		if c, ok := f.cards[id.ID]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// fakeStore stands in for the database while legality still reads Scryfall.
type fakeStore struct{}

func (fakeStore) DeckValues(ctx context.Context, userID string, deckID string) (DeckValues, error) {
	return DeckValues{}, nil
}

func (fakeStore) Printings(ctx context.Context, scryfallIDs []string) (map[string]Printing, error) {
	return map[string]Printing{}, nil
}

func ptr(i int) *int { return &i }

// --- GetDeckLegality ------------------------------------------------------

func TestGetDeckLegality_UnknownDeckReturnsErrDeckNotFound(t *testing.T) {
	svc := NewService(&fakeDeckService{}, &fakeScryfallFetcher{}, fakeStore{})

	_, err := svc.GetDeckLegality(context.Background(), testUserID, "00000000-0000-0000-0000-000000000999")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestGetDeckLegality_PropagatesADeckContentError(t *testing.T) {
	boom := errors.New("db down")
	svc := NewService(&fakeDeckService{err: boom}, &fakeScryfallFetcher{}, fakeStore{})

	_, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)

	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrDeckNotFound)
}

func TestGetDeckLegality_AllLegalCardsReportsLegalTrue(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"}, owned("Sol Ring", "aaaa", 10))
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"aaaa": {ID: "aaaa", Name: "Sol Ring", TypeLine: "Artifact", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.True(t, report.Legal)
	assert.Empty(t, report.Issues)
	assert.Equal(t, "commander", report.Format)
}

func TestGetDeckLegality_BannedCardIsReportedAndMarksDeckIllegal(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"}, owned("Channel", "bbbb", 10))
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"bbbb": {ID: "bbbb", Name: "Channel", TypeLine: "Sorcery", Legalities: map[string]string{"commander": "banned"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "banned")
	assert.Equal(t, 10, report.Issues[0].CardID)
}

func TestGetDeckLegality_UnrecognizedFormatReturnsErrUnknownFormat(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "not-a-real-format"}, owned("Sol Ring", "aaaa", 10))
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"aaaa": {ID: "aaaa", Name: "Sol Ring", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	_, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownFormat)
}

func TestGetDeckLegality_CardNotFoundOnScryfallIsReportedAndMarksDeckIllegal(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"}, owned("Ghost Card", "cccc", 10))
	svc := NewService(decks, &fakeScryfallFetcher{}, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "not found on scryfall")
}

func TestGetDeckLegality_CommanderSingletonViolationIsReported(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"}, owned("Sol Ring", "aaaa", 10, 11))
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"aaaa": {ID: "aaaa", Name: "Sol Ring", TypeLine: "Artifact", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "singleton violation: 2 copies")
}

func TestGetDeckLegality_DuplicateBasicLandsAreExemptFromSingleton(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"}, owned("Mountain", "mtn", 10, 11))
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"mtn": {ID: "mtn", Name: "Mountain", TypeLine: "Basic Land — Mountain", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.True(t, report.Legal)
	assert.Empty(t, report.Issues)
}

func TestGetDeckLegality_CardOutsideCommanderColorIdentityIsReported(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"},
		inSlot(deck.SlotCommander, owned("Golos, Tireless Pilgrim", "golos", 10)),
		owned("Lightning Bolt", "bolt", 11),
	)
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"golos": {ID: "golos", Name: "Golos, Tireless Pilgrim", TypeLine: "Legendary Creature — Dog", ColorIdentity: []string{}, Legalities: map[string]string{"commander": "legal"}},
		"bolt":  {ID: "bolt", Name: "Lightning Bolt", TypeLine: "Instant", ColorIdentity: []string{"R"}, Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "outside commander's color identity")
	assert.Equal(t, 11, report.Issues[0].CardID)
}

func TestGetDeckLegality_CardWithinCommanderColorIdentityIsFine(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"},
		inSlot(deck.SlotCommander, owned("Krenko, Mob Boss", "krenko", 10)),
		owned("Lightning Bolt", "bolt", 11),
	)
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"krenko": {ID: "krenko", Name: "Krenko, Mob Boss", TypeLine: "Legendary Creature — Goblin", ColorIdentity: []string{"R"}, Legalities: map[string]string{"commander": "legal"}},
		"bolt":   {ID: "bolt", Name: "Lightning Bolt", TypeLine: "Instant", ColorIdentity: []string{"R"}, Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.True(t, report.Legal)
}

func TestGetDeckLegality_NonCommanderFormatSkipsConstructionRules(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "modern"}, owned("Mountain", "mtn", 10, 11))
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"mtn": {ID: "mtn", Name: "Mountain", TypeLine: "Basic Land — Mountain", Legalities: map[string]string{"modern": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.True(t, report.Legal) // duplicate Mountains are fine outside commander
}

func TestGetDeckLegality_PropagatesScryfallError(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"}, owned("Sol Ring", "aaaa", 10))
	fetcher := &fakeScryfallFetcher{err: errors.New("network down")}
	svc := NewService(decks, fetcher, fakeStore{})

	_, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrScryfallUnavailable)
}

func TestGetDeckLegality_UsesAPendingCommanderAndPendingCards(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander", CommanderPendingID: ptr(5)},
		inSlot(deck.SlotCommander, pending("Golos, Tireless Pilgrim", "golos", 5, 1)),
		owned("Lightning Bolt", "bolt", 11),
		pending("Counterspell", "counter", 6, 1),
	)
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"golos":   {ID: "golos", Name: "Golos, Tireless Pilgrim", TypeLine: "Legendary Creature — Dog", ColorIdentity: []string{}, Legalities: map[string]string{"commander": "legal"}},
		"bolt":    {ID: "bolt", Name: "Lightning Bolt", TypeLine: "Instant", ColorIdentity: []string{"R"}, Legalities: map[string]string{"commander": "legal"}},
		"counter": {ID: "counter", Name: "Counterspell", TypeLine: "Instant", ColorIdentity: []string{"U"}, Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	reason := "outside commander's color identity (card: %s, commander: )"
	assert.ElementsMatch(t, []LegalityIssue{
		{CardID: 11, CardName: "Lightning Bolt", Board: deck.BoardMain, Reason: fmt.Sprintf(reason, "R")},
		{CardName: "Counterspell", Board: deck.BoardMain, Reason: fmt.Sprintf(reason, "U")},
	}, report.Issues)
}

func TestGetDeckLegality_PendingCopiesCountForSingleton(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander"},
		withPending(5, 1, owned("Sol Ring", "ring", 11)),
	)
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"ring": {ID: "ring", Name: "Sol Ring", TypeLine: "Artifact", Legalities: map[string]string{"commander": "legal"}},
	}}
	svc := NewService(decks, fetcher, fakeStore{})

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, report.Issues[0].Reason, "singleton violation: 2 copies")
}

func TestGetDeckLegality_AnIssueOnAPendingOnlyCardHasNoCardID(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "modern"}, pending("Channel", "channel", 5, 2))
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"channel": {ID: "channel", Name: "Channel", TypeLine: "Sorcery", Legalities: map[string]string{"modern": "not_legal"}},
	}}

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	assert.Equal(t, []LegalityIssue{{CardName: "Channel", Board: deck.BoardMain, Reason: "not legal in modern"}}, report.Issues)
}

func TestGetDeckLegality_AnIssueOnAnOwnedCardCarriesAnOwnedCopysID(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "modern"},
		withPending(5, 2, owned("Channel", "channel", 42, 43)),
	)
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"channel": {ID: "channel", Name: "Channel", TypeLine: "Sorcery", Legalities: map[string]string{"modern": "banned"}},
	}}

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)
	report = ignoringDeckSize(report)

	require.NoError(t, err)
	require.Len(t, report.Issues, 1)
	assert.Contains(t, []int{42, 43}, report.Issues[0].CardID)
	assert.Equal(t, "banned in modern", report.Issues[0].Reason)
}

func TestGetDeckLegality_CountsAPendingCardOf1500Copies(t *testing.T) {
	decks := deckWith(deck.Deck{Name: "Pile", Format: "commander", CommanderPendingID: ptr(5)},
		inSlot(deck.SlotCommander, pending("Golos, Tireless Pilgrim", "golos", 5, 1)),
		pending("Wastes", "wastes", 6, 1500),
		pending("Lightning Bolt", "bolt", 7, 1500),
	)
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"golos":  {ID: "golos", Name: "Golos, Tireless Pilgrim", TypeLine: "Legendary Creature — Dog", ColorIdentity: []string{}, Legalities: map[string]string{"commander": "legal"}},
		"wastes": {ID: "wastes", Name: "Wastes", TypeLine: "Basic Land", Legalities: map[string]string{"commander": "legal"}},
		"bolt":   {ID: "bolt", Name: "Lightning Bolt", TypeLine: "Instant", ColorIdentity: []string{"R"}, Legalities: map[string]string{"commander": "legal"}},
	}}

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	assert.ElementsMatch(t, []LegalityIssue{
		{Reason: "deck size: 3001 cards, commander requires exactly 100"},
		{CardName: "Lightning Bolt", Board: deck.BoardMain, Reason: "outside commander's color identity (card: R, commander: )"},
		{CardName: "Lightning Bolt", Reason: "singleton violation: 1500 copies in deck (commander allows only 1, except basic lands)"},
	}, report.Issues)
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

func deckOf(format string, count int) (*fakeDeckService, *fakeScryfallFetcher) {
	ids := make([]int, 0, count)
	for i := 0; i < count; i++ {
		ids = append(ids, i+1)
	}
	decks := deckWith(deck.Deck{Name: "Pile", Format: format}, owned("Forest", "forest", ids...))
	fetcher := &fakeScryfallFetcher{cards: map[string]scryfall.Card{
		"forest": {ID: "forest", Name: "Forest", TypeLine: "Basic Land — Forest", Legalities: map[string]string{format: "legal"}},
	}}
	return decks, fetcher
}

func TestGetDeckLegality_CommanderDeckOfExactlyAHundredCardsIsLegal(t *testing.T) {
	decks, fetcher := deckOf("commander", 100)

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.NoError(t, err)
	assert.True(t, report.Legal)
	assert.Empty(t, report.Issues)
}

func TestGetDeckLegality_CommanderDeckWithTheWrongCardCountIsNotLegal(t *testing.T) {
	for _, count := range []int{99, 101} {
		decks, fetcher := deckOf("commander", count)

		report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)

		require.NoError(t, err)
		assert.False(t, report.Legal)
		require.Len(t, report.Issues, 1)
		assert.Empty(t, report.Issues[0].CardName)
		assert.Equal(t, fmt.Sprintf("deck size: %d cards, commander requires exactly 100", count), report.Issues[0].Reason)
	}
}

func TestGetDeckLegality_ConstructedDeckNeedsAtLeastSixtyCards(t *testing.T) {
	decks, fetcher := deckOf("modern", 59)
	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)
	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 1)
	assert.Equal(t, "deck size: 59 cards, modern requires at least 60", report.Issues[0].Reason)

	decks, fetcher = deckOf("modern", 75)
	report, err = NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)
	require.NoError(t, err)
	assert.True(t, report.Legal)
}

func TestGetDeckLegality_CountsPendingCopiesInTheDeckSize(t *testing.T) {
	decks, fetcher := deckOf("commander", 97)
	decks.add(foil(pending("Forest", "forest", 1, 3)))

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.NoError(t, err)
	assert.True(t, report.Legal)
}

func TestGetDeckLegality_PutsTheDeckSizeIssueFirst(t *testing.T) {
	decks, fetcher := deckOf("commander", 2)
	decks.add(owned("Channel", "channel", 50))
	fetcher.cards["channel"] = scryfall.Card{ID: "channel", Name: "Channel", TypeLine: "Sorcery", Legalities: map[string]string{"commander": "banned"}}

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.NoError(t, err)
	require.Len(t, report.Issues, 2)
	assert.Equal(t, "deck size: 3 cards, commander requires exactly 100", report.Issues[0].Reason)
	assert.Equal(t, "Channel", report.Issues[1].CardName)
}

func TestGetDeckLegality_FormatsWithoutASizeRuleAreNotChecked(t *testing.T) {
	decks, fetcher := deckOf("someformat", 3)

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.NoError(t, err)
	assert.True(t, report.Legal)
}

func TestGetDeckLegality_SideboardAndConsideringStayOutOfTheDeckSizeAndSingleton(t *testing.T) {
	decks, fetcher := deckOf("commander", 100)
	decks.add(
		onBoard(deck.BoardSideboard, owned("Sol Ring", "ring", 500)),
		withPending(1, 2, onBoard(deck.BoardConsidering, owned("Sol Ring", "ring", 501))),
	)
	fetcher.cards["ring"] = scryfall.Card{ID: "ring", Name: "Sol Ring", TypeLine: "Artifact", Legalities: map[string]string{"commander": "legal"}}

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.NoError(t, err)
	assert.True(t, report.Legal, report.Issues)
}

func TestGetDeckLegality_ChecksTheSideboardAndConsideringCards(t *testing.T) {
	decks, fetcher := deckOf("commander", 100)
	decks.add(
		onBoard(deck.BoardSideboard, owned("Channel", "channel", 500)),
		onBoard(deck.BoardConsidering, pending("Channel", "channel", 1, 1)),
	)
	fetcher.cards["channel"] = scryfall.Card{ID: "channel", Name: "Channel", TypeLine: "Sorcery", Legalities: map[string]string{"commander": "banned"}}

	report, err := NewService(decks, fetcher, fakeStore{}).GetDeckLegality(context.Background(), testUserID, testDeckID)

	require.NoError(t, err)
	assert.False(t, report.Legal)
	require.Len(t, report.Issues, 2)
	assert.Equal(t, deck.BoardSideboard, report.Issues[0].Board)
	assert.Equal(t, deck.BoardConsidering, report.Issues[1].Board)
}
