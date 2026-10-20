package bulk

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
)

func bulkEditResolver() *fakeResolver {
	resolver := tamiyoResolver()
	bolt := ResolvedCard{ScryfallID: "aaaaaaaa-0000-0000-0000-000000000005", Name: "Lightning Bolt", SetCode: "m10", CollectorNumber: "146", ManaValue: 1, CardType: "Instant", ColorIdentity: "R", Colors: "R"}
	resolver.resolved[resolveKeyByName("Lightning Bolt")] = bolt
	return resolver
}

func TestBulkEditDeck_KeepsListedCardsRemovesTheRestAndAddsNewOnes(t *testing.T) {
	decks := tamiyoDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, bulkEditResolver())
	list := "1 Tamiyo, Inquisitive Student (NEO) 75 #commander\n" +
		"4 Island (NEO) 294 #land\n" +
		"2 Lightning Bolt #removal\n" +
		"SIDEBOARD:\n" +
		"1 Duress (M19) 94\n"

	summary, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader(list))

	require.NoError(t, err)
	assert.Equal(t, 3, summary.CardsRemoved)
	assert.Equal(t, 2, summary.CardsPending)
	plan := decks.placedPlan(t, fixtureDeckID)
	assert.Equal(t, []int{1}, plan.RemoveCards, "Sol Ring is not on the list")
	assert.ElementsMatch(t, []deck.PendingRemoval{{ID: 9, Count: 1}, {ID: 8, Count: 1}}, plan.PendingRemovals, "the foil Duress being considered is not on the list; two owned Islands and two pending ones make four")
	assert.Equal(t, []placedLine{{name: "Lightning Bolt", setCode: "m10", quantity: 2, board: deck.BoardMain, anyPrinting: true}}, placedLines(plan))
	assert.Empty(t, plan.Tags, "bulk edit replaces the tags itself")
	assert.Equal(t, []deck.TaggedCard{
		{Name: "Tamiyo, Inquisitive Student", Tags: []string{"commander"}},
		{Name: "Island", Tags: []string{"land"}},
		{Name: "Lightning Bolt", Tags: []string{"removal"}},
	}, plan.ReplaceTags, "removals, placements and tags go in one plan")
}

func TestBulkEditDeck_TheCurrentListChangesNothing(t *testing.T) {
	decks := tamiyoDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{err: assert.AnError})
	var exported bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Format: DeckExportMoxfield, WithTags: true}, &exported))

	summary, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, &exported)

	require.NoError(t, err, "nothing to add, so Scryfall isn't needed")
	assert.Empty(t, decks.placed)
	assert.Zero(t, summary.CardsRemoved)
}

func TestBulkEditDeck_MovesACardToAnotherBoardAndClearsTags(t *testing.T) {
	decks := tamiyoDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, bulkEditResolver())
	list := "1 Tamiyo, Inquisitive Student (NEO) 75\n5 Island (NEO) 294\nSIDEBOARD:\n1 Sol Ring\n1 Duress (M19) 94\nMAYBEBOARD:\n1 Duress (M19) 94 *F*\n"

	_, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader(list))

	require.NoError(t, err)
	plan := decks.placedPlan(t, fixtureDeckID)
	assert.Equal(t, []int{1}, plan.RemoveCards, "Sol Ring leaves the main board")
	assert.Equal(t, []deck.TaggedCard{{Name: "Sol Ring", Tags: []string{}}}, plan.ReplaceTags, "Sol Ring had a tag the list no longer gives it")
}

func TestBulkEditDeck_ClearsARemovedCommander(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.decks[0].CommanderPendingID = nil
	decks.decks[0].CommanderID = ptr(1)
	entries := decks.entries[fixtureDeckID]
	entries[0].Slot = deck.SlotNone
	entries[2].Slot = deck.SlotCommander
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, bulkEditResolver())

	_, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader("5 Island (NEO) 294\n"))

	require.NoError(t, err)
	assert.Contains(t, decks.placedPlan(t, fixtureDeckID).RemoveCards, 1, "the deck module clears the slot of a removed commander")
}

func TestBulkEditDeck_Errors(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), &fakeResolver{err: assert.AnError})

	_, err := svc.BulkEditDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000099", strings.NewReader("1 Island\n"))
	assert.ErrorIs(t, err, ErrDeckNotFound)
	_, err = svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader("not a card line\n"))
	assert.ErrorIs(t, err, ErrInvalidFile)

	decks := tamiyoDeckFixture()
	svc = NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{err: assert.AnError})
	_, err = svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader("1 Lightning Bolt\n"))
	assert.ErrorIs(t, err, ErrScryfallUnavailable)
	assert.Empty(t, decks.placed, "nothing is removed when the new cards can't be looked up")
}

func TestExportDeck_MoxfieldWithTags(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Format: DeckExportMoxfield, WithTags: true}, &buf))

	assert.Contains(t, buf.String(), "1 Sol Ring (sld) 1011 #Ramp\n")
}

func TestHandler_BulkEditDeck(t *testing.T) {
	service := &fakeImportService{summary: Summary{CardsRemoved: 2, CardsPending: 1}}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, multipartRequest(t, "/deck/00000000-0000-0000-0000-000000000001/bulk-edit", "1 Island\n", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"cards_removed":2`)
	assert.Equal(t, "1 Island\n", service.lastFileContent)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", service.lastDeckID)

	cases := map[error]int{ErrDeckNotFound: http.StatusNotFound, ErrInvalidFile: http.StatusBadRequest, ErrScryfallUnavailable: http.StatusBadGateway}
	for editErr, want := range cases {
		w := httptest.NewRecorder()
		setupRouter(&fakeImportService{err: editErr}).ServeHTTP(w, multipartRequest(t, "/deck/00000000-0000-0000-0000-000000000001/bulk-edit", "1 Island\n", nil))
		assert.Equal(t, want, w.Code, editErr.Error())
	}
}

func mergedContentDeck() *fakeDeckService {
	sol := ownedExportEntry("Sol Ring", "sld", "1011", 1)
	sol.ScryfallID = solRingSLD
	solCopy := ownedExportEntry("Sol Ring", "sld", "1011", 2)
	solCopy.ScryfallID = solRingSLD
	island := ownedExportEntry("Island", "neo", "294", 3)
	island.ScryfallID = islandNEO
	island.Pending = []deck.PendingPart{{ID: 8, Quantity: 2}}
	islandProxy := ownedExportEntry("Island", "neo", "294", 4)
	islandProxy.ScryfallID = islandNEO
	islandProxy.Proxy = true
	return contentDeck(deck.Deck{Name: "Merged", Format: "commander", CommanderID: ptr(1)}, asCommander(sol), solCopy, island, islandProxy)
}

func TestBulkEditDeck_TheExportOfAMergedDeckChangesNothing(t *testing.T) {
	decks := mergedContentDeck()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{err: assert.AnError})
	var exported bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Format: DeckExportMoxfield}, &exported))

	summary, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, &exported)

	require.NoError(t, err, "the export lists every card the deck has, so nothing is looked up")
	assert.Empty(t, decks.placed)
	assert.Zero(t, summary.CardsRemoved)
}

func TestBulkEditDeck_ShrinksAPendingCommanderSplitAcrossEntries(t *testing.T) {
	commander := asCommander(pendingExportEntry("Tamiyo, Inquisitive Student", "neo", "75", 7, 1))
	extra := pendingExportEntry("Tamiyo, Inquisitive Student", "neo", "75", 7, 2)
	extra.Pending = append(extra.Pending, deck.PendingPart{ID: 11, Quantity: 1})
	decks := contentDeck(deck.Deck{Name: "Tamiyo", Format: "commander", CommanderPendingID: ptr(7)}, commander, extra)
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{err: assert.AnError})

	summary, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader("2 Tamiyo, Inquisitive Student (NEO) 75\n"))

	require.NoError(t, err)
	assert.ElementsMatch(t, []deck.PendingRemoval{{ID: 7, Count: 1}, {ID: 11, Count: 1}}, decks.placedPlan(t, fixtureDeckID).PendingRemovals, "the commander keeps its slot and one extra copy")
	assert.Equal(t, 2, summary.CardsRemoved)
}

func TestBulkEditDeck_AFailedPlacementReportsNoPartialSummary(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.placeErr = assert.AnError
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, bulkEditResolver())

	summary, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader("5 Island (NEO) 294\n"))

	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, Summary{}, summary)
}
