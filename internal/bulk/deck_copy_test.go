package bulk

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

const (
	fixtureDeckID = "00000000-0000-0000-0000-000000000001"
	otherUserID   = "22222222-2222-2222-2222-222222222222"
)

func sharedDeckFixture() *fakeDeckService {
	decks := tamiyoDeckFixture()
	decks.nextID = 50
	decks.owners = map[string]string{fixtureDeckID: otherUserID}
	return decks
}

func TestDuplicateDeck_AsksTheDeckModuleForTheCopyAndSummarisesIt(t *testing.T) {
	decks := &fakeDeckService{duplicateResult: deck.DuplicateResult{
		Deck: deck.Deck{ID: "00000000-0000-0000-0000-000000000050"},
		Placement: deck.PlacementSummary{
			CardsLinked: 2, CardsPending: 7,
			SkippedTags: []deck.SkippedTags{{CardName: "Sol Ring", Err: deck.ErrCardNotInDeck}},
		},
	}}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, nil)
	folderID := 9

	result, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{Name: "Tempo v2", FolderID: &folderID, Visibility: deck.VisibilityPublic})

	require.NoError(t, err)
	assert.Equal(t, []duplication{{sourceID: fixtureDeckID, req: deck.DuplicateRequest{Name: "Tempo v2", FolderID: &folderID, Visibility: deck.VisibilityPublic}}}, decks.duplicated)
	assert.Equal(t, "00000000-0000-0000-0000-000000000050", result.DeckID)
	assert.Equal(t, Summary{
		DecksCreated: 1, CardsLinked: 2, CardsPending: 7,
		Warnings: []string{`could not tag "Sol Ring": card is not in the deck`},
	}, result.Summary)
}

func TestDuplicateDeck_UnknownDeck(t *testing.T) {
	decks := &fakeDeckService{duplicateErr: deck.ErrNotFound}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, nil)

	_, err := svc.DuplicateDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000099", DuplicateRequest{})

	assert.ErrorIs(t, err, deck.ErrNotFound)
}

func TestDuplicateDeck_ReturnsTheErrorWithoutASummary(t *testing.T) {
	failure := errors.New("database down")
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, &fakeDeckService{duplicateErr: failure}, nil)

	result, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{})

	assert.ErrorIs(t, err, failure)
	assert.Equal(t, DuplicateSummary{}, result)
}

func TestCollectDeck_PendingOnOwnDeckAsksTheDeckModuleForThePendingCardsOnly(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.collectResult = deck.CollectSummary{CardsCreated: 4, CardsLinked: 4}
	storages := &fakeStorageService{storages: []storage.Storage{{ID: 4, Name: "Box", Type: "box"}}}
	svc := NewService(&fakeCardService{}, storages, decks, &fakeResolver{})

	summary, err := svc.CollectDeck(context.Background(), testUserID, fixtureDeckID, CollectRequest{
		Mode: CollectPending, StorageID: ptr(4), Boards: []string{deck.BoardSideboard, deck.BoardMain},
	})

	require.NoError(t, err)
	assert.Equal(t, Summary{CardsCreated: 4, CardsLinked: 4}, summary)
	assert.Equal(t, []deck.OwnDeckCollect{{Boards: []string{deck.BoardMain, deck.BoardSideboard}, StorageID: ptr(4)}}, decks.ownCollects)
	assert.Empty(t, decks.sharedCollects)
}

func TestCollectDeck_AllOnOwnDeckAlsoAsksForCopiesOfTheOwnedCards(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.collectResult = deck.CollectSummary{CardsCreated: 7, CardsLinked: 4}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.CollectDeck(context.Background(), testUserID, fixtureDeckID, CollectRequest{Mode: CollectAll, Boards: []string{deck.BoardMain}})

	require.NoError(t, err)
	assert.Equal(t, Summary{CardsCreated: 7, CardsLinked: 4}, summary)
	assert.Equal(t, []deck.OwnDeckCollect{{Boards: []string{deck.BoardMain}, OwnedCopies: true}}, decks.ownCollects)
}

func TestCollectDeck_OnSomeoneElsesDeckUsesTheSharedCollect(t *testing.T) {
	for mode, missingOnly := range map[string]bool{CollectAll: false, CollectMissing: true} {
		decks := sharedDeckFixture()
		decks.collectResult = deck.CollectSummary{CardsCreated: 9}
		svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

		summary, err := svc.CollectDeck(context.Background(), testUserID, fixtureDeckID, CollectRequest{
			Mode: mode, Boards: []string{deck.BoardConsidering, deck.BoardMain},
		})

		require.NoError(t, err)
		assert.Equal(t, Summary{CardsCreated: 9}, summary)
		assert.Equal(t, []deck.SharedDeckCollect{{Boards: []string{deck.BoardMain, deck.BoardConsidering}, MissingOnly: missingOnly}}, decks.sharedCollects)
		assert.Empty(t, decks.ownCollects)
	}
}

func TestCollectDeck_ReadsTheDeckAsVisibleToTheViewer(t *testing.T) {
	decks := sharedDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.CollectDeck(context.Background(), testUserID, fixtureDeckID, CollectRequest{Mode: CollectAll, Boards: []string{deck.BoardMain}})

	require.NoError(t, err)
	assert.Equal(t, []deckRead{{viewerID: testUserID, deckID: fixtureDeckID, reach: deck.ReachVisible}}, decks.reads)
}

func TestCollectDeck_ADeckOutOfReachIsNotFound(t *testing.T) {
	decks := sharedDeckFixture()
	decks.readErr = deck.ErrNotFound
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	_, err := svc.CollectDeck(context.Background(), testUserID, fixtureDeckID, CollectRequest{Mode: CollectAll, Boards: []string{deck.BoardMain}})

	assert.ErrorIs(t, err, deck.ErrNotFound)
	assert.Empty(t, decks.sharedCollects)
	assert.Empty(t, decks.ownCollects)
}

func TestCollectDeck_ReturnsTheDeckModuleFailureWithoutASummary(t *testing.T) {
	decks := tamiyoDeckFixture()
	decks.collectErr = errors.New("boom")
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{})

	summary, err := svc.CollectDeck(context.Background(), testUserID, fixtureDeckID, CollectRequest{Mode: CollectAll, Boards: []string{deck.BoardMain}})

	assert.ErrorIs(t, err, decks.collectErr)
	assert.Equal(t, Summary{}, summary)
}

func TestCollectDeck_RejectsBadRequests(t *testing.T) {
	ctx := context.Background()
	own := NewService(&fakeCardService{}, &fakeStorageService{}, tamiyoDeckFixture(), &fakeResolver{})
	shared := NewService(&fakeCardService{}, &fakeStorageService{}, sharedDeckFixture(), &fakeResolver{})
	main := []string{deck.BoardMain}

	_, err := own.CollectDeck(ctx, testUserID, fixtureDeckID, CollectRequest{Mode: "everything", Boards: main})
	assert.ErrorIs(t, err, ErrUnknownCollectMode)
	_, err = own.CollectDeck(ctx, testUserID, fixtureDeckID, CollectRequest{Mode: CollectAll})
	assert.ErrorIs(t, err, ErrNoBoards)
	_, err = own.CollectDeck(ctx, testUserID, fixtureDeckID, CollectRequest{Mode: CollectAll, Boards: []string{"maybe"}})
	assert.ErrorIs(t, err, ErrNoBoards)
	_, err = own.CollectDeck(ctx, testUserID, fixtureDeckID, CollectRequest{Mode: CollectMissing, Boards: main})
	assert.ErrorIs(t, err, ErrCollectModeNotHere)
	_, err = shared.CollectDeck(ctx, testUserID, fixtureDeckID, CollectRequest{Mode: CollectPending, Boards: main})
	assert.ErrorIs(t, err, ErrCollectModeNotHere)
	_, err = own.CollectDeck(ctx, testUserID, "00000000-0000-0000-0000-000000000099", CollectRequest{Mode: CollectAll, Boards: main})
	assert.ErrorIs(t, err, deck.ErrNotFound)
}

func ptrString(s string) *string { return &s }

func postJSON(router http.Handler, path string, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHandler_DuplicateDeck(t *testing.T) {
	service := &fakeImportService{duplicateResult: DuplicateSummary{DeckID: "00000000-0000-0000-0000-000000000042", Summary: Summary{DecksCreated: 1, CardsPending: 3}}}

	w := postJSON(setupRouter(service), "/deck/00000000-0000-0000-0000-000000000009/duplicate", "")

	require.Equal(t, http.StatusCreated, w.Code)
	assert.JSONEq(t, `{"deck_id": "00000000-0000-0000-0000-000000000042", "summary": {"cards_created": 0, "cards_linked": 0, "cards_pending": 3, "cards_skipped": 0, "storages_created": 0, "decks_created": 1}}`, w.Body.String())
	assert.Equal(t, "00000000-0000-0000-0000-000000000009", service.lastDeckID)
	assert.Equal(t, testUserID, service.lastUserID)
	assert.Equal(t, DuplicateRequest{}, service.lastDuplicate)
}

func TestHandler_DuplicateDeck_PassesTheBody(t *testing.T) {
	service := &fakeImportService{}

	w := postJSON(setupRouter(service), "/deck/00000000-0000-0000-0000-000000000009/duplicate", `{"name": "X", "folder_id": 3, "visibility": "public"}`)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, DuplicateRequest{Name: "X", FolderID: ptr(3), Visibility: "public"}, service.lastDuplicate)
}

func TestHandler_DuplicateDeck_MapsErrors(t *testing.T) {
	router := setupRouter(&fakeImportService{})
	assert.Equal(t, http.StatusBadRequest, postJSON(router, "/deck/00000000-0000-0000-0000-000000000009/duplicate", `{"folder_id": 0}`).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, "/deck/00000000-0000-0000-0000-000000000009/duplicate", `{"visibility": "secret"}`).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, "/deck/00000000-0000-0000-0000-000000000009/duplicate", `{`).Code)

	cases := map[error]int{deck.ErrNotFound: http.StatusNotFound, deck.ErrTargetFolderNotFound: http.StatusBadRequest}
	for duplicateErr, expected := range cases {
		w := postJSON(setupRouter(&fakeImportService{err: duplicateErr}), "/deck/00000000-0000-0000-0000-000000000009/duplicate", "")
		assert.Equal(t, expected, w.Code)
	}
	w := postJSON(setupRouter(&fakeImportService{}), "/deck/nope/duplicate", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_CollectDeck(t *testing.T) {
	service := &fakeImportService{summary: Summary{CardsCreated: 5}}

	w := postJSON(setupRouter(service), "/deck/00000000-0000-0000-0000-000000000009/collect", `{"mode": "missing", "storage_id": 4, "boards": ["main", "sideboard"]}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"cards_created":5`)
	assert.Equal(t, CollectRequest{Mode: CollectMissing, StorageID: ptr(4), Boards: []string{"main", "sideboard"}}, service.lastCollect)
}

func TestHandler_CollectDeck_RejectsBadInput(t *testing.T) {
	router := setupRouter(&fakeImportService{})
	assert.Equal(t, http.StatusBadRequest, postJSON(router, "/deck/00000000-0000-0000-0000-000000000009/collect", "").Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(router, "/deck/00000000-0000-0000-0000-000000000009/collect", `{"mode": "all", "storage_id": 0, "boards": ["main"]}`).Code)

	cases := map[error]int{
		deck.ErrNotFound:              http.StatusNotFound,
		ErrUnknownCollectMode:         http.StatusBadRequest,
		ErrCollectModeNotHere:         http.StatusBadRequest,
		ErrNoBoards:                   http.StatusBadRequest,
		deck.ErrTargetStorageNotFound: http.StatusBadRequest,
	}
	for collectErr, expected := range cases {
		w := postJSON(setupRouter(&fakeImportService{err: collectErr}), "/deck/00000000-0000-0000-0000-000000000009/collect", `{"mode": "all", "boards": ["main"]}`)
		assert.Equal(t, expected, w.Code, collectErr.Error())
	}
}
