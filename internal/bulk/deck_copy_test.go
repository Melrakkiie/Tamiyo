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

func copyDeckFixture() *fakeDeckService {
	decks := tamiyoDeckFixture()
	decks.nextID = 50
	return decks
}

func sharedDeckFixture() *fakeDeckService {
	decks := copyDeckFixture()
	decks.owners = map[string]string{fixtureDeckID: otherUserID}
	return decks
}

func TestDuplicateDeck_CopiesOwnDeckAsPrivate(t *testing.T) {
	decks := copyDeckFixture()
	decks.decks[0].BackgroundScryfallID = ptrString(solRingSLD)
	decks.decks[0].Visibility = deck.VisibilityPublic
	bracket := 3
	decks.decks[0].Bracket = &bracket
	folderID := 7
	decks.decks[0].FolderID = &folderID
	decks.decks[0].Favorite = true
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver())
	targetFolderID := 9

	result, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{Name: "  Tempo v2 ", FolderID: &targetFolderID})

	require.NoError(t, err)
	require.Len(t, decks.created, 1)
	copied := decks.created[0]
	assert.Equal(t, copied.ID, result.DeckID)
	assert.Equal(t, "Tempo v2", copied.Name)
	assert.Equal(t, "commander", copied.Format)
	assert.Equal(t, deck.VisibilityPrivate, copied.Visibility)
	assert.Equal(t, ptrString(solRingSLD), copied.BackgroundScryfallID)
	assert.Equal(t, &bracket, copied.Bracket)
	assert.Equal(t, &targetFolderID, copied.FolderID)
	assert.False(t, copied.Favorite)
	assert.Equal(t, 1, result.Summary.DecksCreated)
	assert.Equal(t, 9, result.Summary.CardsPending)
	plan := decks.placedPlan(t, result.DeckID)
	assert.Equal(t, "Tamiyo, Inquisitive Student", plan.Lines[0].Printing.Name)
	assert.True(t, plan.Lines[0].Commander)
	assert.Equal(t, []deck.TaggedCard{{Name: "Sol Ring", Tags: []string{"Ramp"}}}, plan.Tags)
}

func TestDuplicateDeck_EmptyRequestGivesDefaultNameAndNoFolder(t *testing.T) {
	decks := copyDeckFixture()
	folderID := 7
	decks.decks[0].FolderID = &folderID
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver())

	_, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{Name: "   "})

	require.NoError(t, err)
	require.Len(t, decks.created, 1)
	assert.Equal(t, "Tamiyo tempo (copie)", decks.created[0].Name)
	assert.Nil(t, decks.created[0].FolderID)
	assert.Equal(t, deck.VisibilityPrivate, decks.created[0].Visibility)
}

func TestDuplicateDeck_UsesTheChosenVisibility(t *testing.T) {
	decks := copyDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver())

	_, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{Visibility: deck.VisibilityUnlisted})

	require.NoError(t, err)
	require.Len(t, decks.created, 1)
	assert.Equal(t, deck.VisibilityUnlisted, decks.created[0].Visibility)
}

func TestDuplicateDeck_CopiesSomeoneElsesSharedDeck(t *testing.T) {
	decks := sharedDeckFixture()
	folderID := 7
	decks.decks[0].FolderID = &folderID
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver())

	result, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{})

	require.NoError(t, err)
	require.Len(t, decks.created, 1)
	assert.Equal(t, "Tamiyo tempo (copie)", decks.created[0].Name)
	assert.Nil(t, decks.created[0].FolderID)
	assert.Equal(t, 9, result.Summary.CardsPending)
}

func TestDuplicateDeck_KeepsOwnedAndPendingProxiesAsProxies(t *testing.T) {
	decks := proxyDeckFixture()
	decks.nextID = 50
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver())

	result, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{})

	require.NoError(t, err)
	assert.Equal(t, []placedLine{
		{name: "Sol Ring", setCode: "sld", quantity: 1, board: deck.BoardMain, proxy: deck.RealOnly},
		{name: "Sol Ring", setCode: "sld", quantity: 2, board: deck.BoardMain, proxy: deck.ProxyOnly},
		{name: "Duress", setCode: "m19", quantity: 2, board: deck.BoardSideboard, proxy: deck.ProxyOnly},
	}, placedLines(decks.placedPlan(t, result.DeckID)))
}

func TestDuplicateDeck_EmptyDeckGivesAnEmptyCopy(t *testing.T) {
	decks := &fakeDeckService{nextID: 50, decks: []deck.Deck{{ID: fixtureDeckID, Name: "Vide", Format: "modern"}}}
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver())

	result, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{})

	require.NoError(t, err)
	require.Len(t, decks.created, 1)
	assert.Equal(t, "Vide (copie)", decks.created[0].Name)
	assert.Equal(t, Summary{DecksCreated: 1}, result.Summary)
}

func TestDuplicateDeck_RemovesTheCopyWhenScryfallFails(t *testing.T) {
	decks := copyDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, &fakeResolver{err: errors.New("down")})

	_, err := svc.DuplicateDeck(context.Background(), testUserID, fixtureDeckID, DuplicateRequest{})

	assert.ErrorIs(t, err, ErrScryfallUnavailable)
	require.Len(t, decks.created, 1)
	assert.Equal(t, []string{decks.created[0].ID}, decks.deleted)
}

func TestDuplicateDeck_UnknownDeck(t *testing.T) {
	decks := tamiyoDeckFixture()
	svc := NewService(&fakeCardService{}, &fakeStorageService{}, decks, tamiyoResolver())

	_, err := svc.DuplicateDeck(context.Background(), testUserID, "00000000-0000-0000-0000-000000000099", DuplicateRequest{})

	assert.ErrorIs(t, err, ErrDeckNotFound)
	assert.Empty(t, decks.created)
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
	_, err = own.CollectDeck(ctx, testUserID, fixtureDeckID, CollectRequest{Mode: CollectAll, Boards: main, StorageID: ptr(9)})
	assert.ErrorIs(t, err, ErrTargetStorageNotFound)
	_, err = own.CollectDeck(ctx, testUserID, "00000000-0000-0000-0000-000000000099", CollectRequest{Mode: CollectAll, Boards: main})
	assert.ErrorIs(t, err, ErrDeckNotFound)
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

	cases := map[error]int{ErrDeckNotFound: http.StatusNotFound, ErrScryfallUnavailable: http.StatusBadGateway, deck.ErrTargetFolderNotFound: http.StatusBadRequest}
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
		ErrDeckNotFound:          http.StatusNotFound,
		ErrUnknownCollectMode:    http.StatusBadRequest,
		ErrCollectModeNotHere:    http.StatusBadRequest,
		ErrNoBoards:              http.StatusBadRequest,
		ErrTargetStorageNotFound: http.StatusBadRequest,
	}
	for collectErr, expected := range cases {
		w := postJSON(setupRouter(&fakeImportService{err: collectErr}), "/deck/00000000-0000-0000-0000-000000000009/collect", `{"mode": "all", "boards": ["main"]}`)
		assert.Equal(t, expected, w.Code, collectErr.Error())
	}
}
