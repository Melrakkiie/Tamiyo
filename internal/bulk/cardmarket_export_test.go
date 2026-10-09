package bulk

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cardmarketExport(t *testing.T, resolver *fakeResolver, opts DeckExportOptions) string {
	t.Helper()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), resolver)
	var buf bytes.Buffer
	require.NoError(t, svc.ExportCardmarketDeck(context.Background(), testUserID, fixtureDeckID, opts, &buf))
	return buf.String()
}

func TestExportCardmarketDeck_WholeDeckByName(t *testing.T) {
	got := cardmarketExport(t, &fakeResolver{}, DeckExportOptions{Format: DeckExportCardmarket})

	assert.Equal(t, "1 Duress\n5 Island\n1 Sol Ring\n1 Tamiyo, Inquisitive Student\n", got)
}

func TestExportCardmarketDeck_OnlyPendingCards(t *testing.T) {
	got := cardmarketExport(t, &fakeResolver{}, DeckExportOptions{Format: DeckExportCardmarket, OnlyPending: true})

	assert.Equal(t, "3 Island\n1 Tamiyo, Inquisitive Student\n", got)
}

func TestExportCardmarketDeck_WithExpansionNames(t *testing.T) {
	resolver := &fakeResolver{setNames: map[string]string{"neo": "Kamigawa: Neon Dynasty", "sld": "Secret Lair Drop"}}

	got := cardmarketExport(t, resolver, DeckExportOptions{Format: DeckExportCardmarket, Printings: true})

	assert.Equal(t, "1 Duress\n5 Island (Kamigawa: Neon Dynasty)\n1 Sol Ring (Secret Lair Drop)\n1 Tamiyo, Inquisitive Student (Kamigawa: Neon Dynasty)\n", got)
}

func TestExportCardmarketDeck_ChosenBoards(t *testing.T) {
	assert.Equal(t, "2 Duress\n", cardmarketExport(t, &fakeResolver{}, DeckExportOptions{Boards: []string{"sideboard", "considering"}}))
	assert.Equal(t, "1 Duress\n", cardmarketExport(t, &fakeResolver{}, DeckExportOptions{Boards: []string{"considering"}, OnlyPending: true}))
	assert.Equal(t, "5 Island\n1 Sol Ring\n1 Tamiyo, Inquisitive Student\n", cardmarketExport(t, &fakeResolver{}, DeckExportOptions{Boards: []string{"main"}}))

	svc := NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), &fakeResolver{})
	err := svc.ExportCardmarketDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Boards: []string{"maybe"}}, &bytes.Buffer{})
	assert.ErrorIs(t, err, ErrNoBoards)
}

func TestExportCardmarketDeck_WritesSplitCardsWithTwoSlashes(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.pending = nil
	decks.cardsByDeck[fixtureDeckID] = append(decks.cardsByDeck[fixtureDeckID][:0], decks.cardsByDeck[fixtureDeckID][0])
	decks.cardsByDeck[fixtureDeckID][0].Name = "Fire / Ice"
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportCardmarketDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{}, &buf))

	assert.Equal(t, "1 Fire // Ice\n", buf.String())
}

func TestExportCardmarketDeck_Errors(t *testing.T) {
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), &fakeResolver{err: errors.New("down")})

	err := svc.ExportCardmarketDeck(context.Background(), testUserID, fixtureDeckID, DeckExportOptions{Printings: true}, &bytes.Buffer{})
	assert.ErrorIs(t, err, ErrScryfallUnavailable)

	err = svc.ExportCardmarketDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000099", DeckExportOptions{}, &bytes.Buffer{})
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestExportSharedDeck_Cardmarket(t *testing.T) {
	decks := sharedDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	var buf bytes.Buffer
	require.NoError(t, svc.ExportSharedDeck(context.Background(), fixtureDeckID, DeckExportOptions{Format: DeckExportCardmarket, OnlyPending: true}, &buf))

	assert.Equal(t, "3 Island\n1 Tamiyo, Inquisitive Student\n", buf.String())
}

func TestHandler_ExportDeck_Cardmarket(t *testing.T) {
	service := &fakeImportService{exportContent: "4 Dark Ritual\n"}
	router := setupRouter(service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/export?format=cardmarket&pending=true&printings=true", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "4 Dark Ritual\n", w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Disposition"), "Deck_cardmarket.txt")
	assert.Equal(t, DeckExportOptions{Format: DeckExportCardmarket, OnlyPending: true, Printings: true}, service.lastExportOptions)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/export?format=cardmarket&boards=main,considering", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"main", "considering"}, service.lastExportOptions.Boards)
	assert.Equal(t, testUserID, service.lastExportUser)

	for _, query := range []string{"pending=maybe", "printings=2x"} {
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/export?format=cardmarket&"+query, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code, query)
	}

	w = httptest.NewRecorder()
	setupRouter(&fakeImportService{exportErr: ErrNoBoards}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/export?format=cardmarket&boards=maybe", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	setupRouter(&fakeImportService{exportErr: ErrScryfallUnavailable}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/deck/00000000-0000-0000-0000-000000000001/export?format=cardmarket&printings=true", nil))
	assert.Equal(t, http.StatusBadGateway, w.Code)
}
