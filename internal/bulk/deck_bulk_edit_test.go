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
	assert.Equal(t, []int{1}, decks.unlinkedCards, "Sol Ring is not on the list")
	assert.Equal(t, []int{9}, decks.removedPending, "the foil Duress being considered is not on the list")
	assert.Equal(t, map[int]int{8: 2}, decks.pendingQuantities, "two owned Islands and two pending ones make four")
	assert.False(t, decks.commanderCleared)
	assert.Equal(t, 3, summary.CardsRemoved)
	assert.Equal(t, 2, summary.CardsPending)
	require.Len(t, decks.addedPending, 1)
	assert.Equal(t, "Lightning Bolt", decks.addedPending[0].Name)
	assert.Equal(t, 2, decks.addedPending[0].Quantity)
	assert.Equal(t, deck.BoardMain, decks.addedPending[0].Board)
	assert.Equal(t, map[string][]string{
		"Tamiyo, Inquisitive Student": {"commander"},
		"Island":                      {"land"},
		"Lightning Bolt":              {"removal"},
	}, decks.setTags)
}

func TestBulkEditDeck_TheCurrentListChangesNothing(t *testing.T) {
	decks := tamiyoDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{err: assert.AnError})
	var exported bytes.Buffer
	require.NoError(t, svc.ExportDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Format: DeckExportMoxfield, WithTags: true}, &exported))

	summary, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, &exported)

	require.NoError(t, err, "nothing to add, so Scryfall isn't needed")
	assert.Empty(t, decks.unlinkedCards)
	assert.Empty(t, decks.removedPending)
	assert.Empty(t, decks.pendingQuantities)
	assert.Empty(t, decks.addedPending)
	assert.Empty(t, decks.setTags)
	assert.Zero(t, summary.CardsRemoved)
}

func TestBulkEditDeck_MovesACardToAnotherBoardAndClearsTags(t *testing.T) {
	decks := tamiyoDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, bulkEditResolver())
	list := "1 Tamiyo, Inquisitive Student (NEO) 75\n5 Island (NEO) 294\nSIDEBOARD:\n1 Sol Ring\n1 Duress (M19) 94\nMAYBEBOARD:\n1 Duress (M19) 94 *F*\n"

	_, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader(list))

	require.NoError(t, err)
	assert.Equal(t, []int{1}, decks.unlinkedCards, "Sol Ring leaves the main board")
	assert.Equal(t, map[string][]string{"Sol Ring": {}}, decks.setTags, "Sol Ring had a tag the list no longer gives it")
}

func TestBulkEditDeck_ClearsARemovedCommander(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.decks[0].CommanderPendingID = nil
	decks.decks[0].CommanderID = ptr(1)
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, bulkEditResolver())

	_, err := svc.BulkEditDeck(context.Background(), testUserID, fixtureDeckID, strings.NewReader("5 Island (NEO) 294\n"))

	require.NoError(t, err)
	assert.True(t, decks.commanderCleared)
	assert.Contains(t, decks.unlinkedCards, 1)
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
	assert.Empty(t, decks.unlinkedCards, "nothing is removed when the new cards can't be looked up")
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
