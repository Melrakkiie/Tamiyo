package deckshare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/user"
)

const (
	viewerID   = "33333333-3333-3333-3333-333333333333"
	strangerID = "44444444-4444-4444-4444-444444444444"
	myDeckID   = "55555555-5555-5555-5555-555555555555"
	theirDeck  = "66666666-6666-6666-6666-666666666666"
	privateID  = "77777777-7777-7777-7777-777777777777"
)

// fakeDeckStore holds the decks as viewerID reads them; any other id is not found.
type fakeDeckStore struct {
	reads   map[string]deck.ReadDeck
	readErr error
	reaches []deck.Reach

	owned       map[string]int
	countedFor  string
	countedKeys []string

	likes map[[2]string]bool
}

func (f *fakeDeckStore) ReadDeck(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (deck.ReadDeck, error) {
	f.reaches = append(f.reaches, reach)
	if f.readErr != nil {
		return deck.ReadDeck{}, f.readErr
	}
	read, ok := f.reads[deckID]
	if !ok {
		return deck.ReadDeck{}, deck.ErrNotFound
	}
	return read, nil
}

func (f *fakeDeckStore) CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error) {
	f.countedFor = userID
	f.countedKeys = nameKeys
	counts := map[string]int{}
	for _, key := range nameKeys {
		if n, ok := f.owned[key]; ok {
			counts[key] = n
		}
	}
	return counts, nil
}

func (f *fakeDeckStore) BrowsePublicDecks(ctx context.Context, filter deck.PublicFilter) ([]deck.PublicDeck, int, error) {
	return nil, 0, nil
}

func compareStore() *fakeDeckStore {
	return &fakeDeckStore{reads: map[string]deck.ReadDeck{
		myDeckID: {
			OwnerID: viewerID,
			Mine:    true,
			Deck:    deck.Deck{ID: myDeckID, Name: "Mine", Format: "commander", Visibility: deck.VisibilityPrivate, CommanderID: intPtr(1)},
			Entries: []deck.DeckEntry{
				{Name: "Atraxa, Praetors' Voice", ScryfallID: "atraxa-cmr", ManaValue: 4, Slot: deck.SlotCommander, OwnedCardIDs: []int{1}},
				{Name: "Counterspell", ScryfallID: "cs", Pending: []deck.PendingPart{{ID: 9, Quantity: 2}}},
				{Name: "Fire // Ice", ScryfallID: "fire-mh2", CardType: strPtr("Instant"), Colors: strPtr("UR"), ColorIdentity: strPtr("UR"), OwnedCardIDs: []int{5}},
				{Name: "Island", ScryfallID: "island-a", OwnedCardIDs: []int{4}},
				{Name: "Sol Ring", ScryfallID: "sol-cmm", OwnedCardIDs: []int{3}},
				{Name: "Sol Ring", ScryfallID: "sol-sld", OwnedCardIDs: []int{2}},
			},
		},
		theirDeck: {
			OwnerID: strangerID,
			Deck:    deck.Deck{ID: theirDeck, Name: "Theirs", Format: "commander", Visibility: deck.VisibilityUnlisted},
			Entries: []deck.DeckEntry{
				{Name: "Fire / Ice", ScryfallID: "fire-other", OwnedCardIDs: []int{23}},
				{Name: "Island", ScryfallID: "island-b", OwnedCardIDs: []int{21}},
				{Name: "Island", ScryfallID: "island-c", OwnedCardIDs: []int{22}},
				{Name: "Lightning Bolt", ScryfallID: "bolt", OwnedCardIDs: []int{24}},
				{Name: "Sol Ring", ScryfallID: "sol-lea", OwnedCardIDs: []int{20}},
			},
		},
	}}
}

func compareUsers() *fakeUsers {
	return &fakeUsers{users: map[string]user.User{
		viewerID:   {ID: viewerID, DisplayName: strPtr("Me")},
		strangerID: {ID: strangerID, DisplayName: strPtr("Alice")},
	}}
}

func names(cards []ComparedCard) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.Name)
	}
	return out
}

func TestCompareDecks_SplitsCardsByNameIgnoringPrintings(t *testing.T) {
	store := compareStore()
	svc := NewService(store, compareUsers())

	comparison, err := svc.CompareDecks(context.Background(), viewerID, myDeckID, theirDeck)

	require.NoError(t, err)
	assert.Equal(t, []string{"Fire // Ice", "Island", "Sol Ring"}, names(comparison.Common))
	assert.Equal(t, []string{"Atraxa, Praetors' Voice", "Counterspell"}, names(comparison.OnlyInDeck))
	assert.Equal(t, []string{"Lightning Bolt"}, names(comparison.OnlyInOther))

	island := comparison.Common[1]
	assert.Equal(t, 1, island.Quantity)
	assert.Equal(t, 2, island.OtherQuantity)
	solRing := comparison.Common[2]
	assert.Equal(t, 2, solRing.Quantity)
	assert.Equal(t, 1, solRing.OtherQuantity)
	assert.Equal(t, "Instant", *comparison.Common[0].CardType)
	assert.Equal(t, "UR", *comparison.Common[0].Colors)
	assert.Equal(t, "UR", *comparison.Common[0].ColorIdentity)

	atraxa := comparison.OnlyInDeck[0]
	assert.True(t, atraxa.Commander)
	assert.False(t, atraxa.OtherCommander)
	assert.Equal(t, 4.0, atraxa.ManaValue)
	assert.Equal(t, 2, comparison.OnlyInDeck[1].Quantity)
	assert.Equal(t, 0, comparison.OnlyInOther[0].Quantity)
	assert.Equal(t, 1, comparison.OnlyInOther[0].OtherQuantity)
}

func TestCompareDecks_DescribesBothDecks(t *testing.T) {
	store := compareStore()
	svc := NewService(store, compareUsers())

	comparison, err := svc.CompareDecks(context.Background(), viewerID, myDeckID, theirDeck)

	require.NoError(t, err)
	assert.True(t, comparison.Deck.Mine)
	assert.Equal(t, 7, comparison.Deck.CardCount)
	assert.False(t, comparison.Other.Mine)
	assert.Equal(t, "Alice", *comparison.Other.Owner.DisplayName)
	assert.Equal(t, 5, comparison.Other.CardCount)
	assert.Equal(t, []deck.Reach{deck.ReachVisible, deck.ReachVisible}, store.reaches)
}

func TestCompareDecks_TwoDecksSharedByOthers(t *testing.T) {
	svc := NewService(compareStore(), compareUsers())

	comparison, err := svc.CompareDecks(context.Background(), viewerID, theirDeck, theirDeck)

	require.NoError(t, err)
	assert.False(t, comparison.Deck.Mine)
	assert.Len(t, comparison.Common, 4)
	assert.Empty(t, comparison.OnlyInDeck)
	assert.Empty(t, comparison.OnlyInOther)
}

func TestCompareSharedDecksHandler_NeedsNoViewer(t *testing.T) {
	service := &fakeSharedService{compared: Comparison{
		Deck:  ComparedDeck{Deck: deck.Deck{ID: theirDeck, Name: "Theirs"}},
		Other: ComparedDeck{Deck: deck.Deck{ID: theirDeck, Name: "Theirs"}},
	}}
	router := setupRouter(service)

	w := get(router, "/shared/decks/"+theirDeck+"/compare/"+theirDeck)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", service.lastUserID)
	assert.Equal(t, theirDeck, service.lastOtherID)
	assert.Equal(t, http.StatusNotFound, get(router, "/shared/decks/nope/compare/"+theirDeck).Code)
	assert.Equal(t, http.StatusNotFound, get(setupRouter(&fakeSharedService{err: deck.ErrNotFound}), "/shared/decks/"+theirDeck+"/compare/"+privateID).Code)
}

func TestCompareDecks_PropagatesUnexpectedErrors(t *testing.T) {
	store := compareStore()
	store.readErr = errors.New("db is down")
	svc := NewService(store, compareUsers())

	_, err := svc.CompareDecks(context.Background(), viewerID, myDeckID, theirDeck)

	require.Error(t, err)
	assert.NotErrorIs(t, err, deck.ErrNotFound)
}

func TestCompareDecksHandler_ReturnsTheComparison(t *testing.T) {
	service := &fakeSharedService{compared: Comparison{
		Deck:        ComparedDeck{Deck: deck.Deck{ID: myDeckID, Name: "Mine"}, Mine: true, CardCount: 3},
		Other:       ComparedDeck{Deck: deck.Deck{ID: theirDeck, Name: "Theirs"}, Owner: Owner{ID: strangerID, DisplayName: strPtr("Alice")}, CardCount: 2},
		Common:      []ComparedCard{{Name: "Sol Ring", ScryfallID: "sol", Colors: strPtr(""), Quantity: 1, OtherQuantity: 2}},
		OnlyInDeck:  []ComparedCard{{Name: "Atraxa", Quantity: 1, Commander: true}},
		OnlyInOther: []ComparedCard{},
	}}
	router := setupProtectedRouter(service, viewerID)

	w := get(router, "/deck/"+myDeckID+"/compare/"+theirDeck)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, viewerID, service.lastUserID)
	assert.Equal(t, myDeckID, service.lastDeckID)
	assert.Equal(t, theirDeck, service.lastOtherID)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["deck"].(map[string]any)["mine"])
	assert.Equal(t, "Alice", body["other"].(map[string]any)["owner"].(map[string]any)["display_name"])
	common := body["common"].([]any)[0].(map[string]any)
	assert.Equal(t, "Sol Ring", common["name"])
	assert.Equal(t, 2.0, common["other_quantity"])
	assert.Equal(t, "", common["colors"])
	assert.Nil(t, common["color_identity"])
	assert.Equal(t, true, body["only_in_deck"].([]any)[0].(map[string]any)["commander"])
	assert.Equal(t, []any{}, body["only_in_other"])
}

func TestCompareDecksHandler_Errors(t *testing.T) {
	cases := map[string]struct {
		userID string
		path   string
		err    error
		want   int
	}{
		"unauthenticated": {"", "/deck/" + myDeckID + "/compare/" + theirDeck, nil, http.StatusUnauthorized},
		"invalid deck id": {viewerID, "/deck/nope/compare/" + theirDeck, nil, http.StatusBadRequest},
		"invalid other":   {viewerID, "/deck/" + myDeckID + "/compare/nope", nil, http.StatusBadRequest},
		"not visible":     {viewerID, "/deck/" + myDeckID + "/compare/" + privateID, deck.ErrNotFound, http.StatusNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := setupProtectedRouter(&fakeSharedService{err: tc.err}, tc.userID)

			assert.Equal(t, tc.want, get(router, tc.path).Code)
		})
	}
}

func TestCompareDecks_OnlyComparesTheMainBoards(t *testing.T) {
	store := compareStore()
	mine := store.reads[myDeckID]
	mine.Entries = append(mine.Entries,
		deck.DeckEntry{Name: "Lightning Bolt", ScryfallID: "bolt", Board: deck.BoardSideboard, OwnedCardIDs: []int{6}},
		deck.DeckEntry{Name: "Opt", ScryfallID: "opt", Board: deck.BoardConsidering, Pending: []deck.PendingPart{{ID: 10, Quantity: 1}}},
	)
	store.reads[myDeckID] = mine
	svc := NewService(store, compareUsers())

	comparison, err := svc.CompareDecks(context.Background(), viewerID, myDeckID, theirDeck)

	require.NoError(t, err)
	assert.Equal(t, []string{"Lightning Bolt"}, names(comparison.OnlyInOther))
	assert.Equal(t, []string{"Atraxa, Praetors' Voice", "Counterspell"}, names(comparison.OnlyInDeck))
	assert.Equal(t, 7, comparison.Deck.CardCount)
}

func TestCollectionOwnership_CountsTheViewersCopiesByName(t *testing.T) {
	store := compareStore()
	store.owned = map[string]int{"sol ring": 3, "fire / ice": 1}
	svc := NewService(store, compareUsers())

	owned, err := svc.CollectionOwnership(context.Background(), viewerID, theirDeck)

	require.NoError(t, err)
	assert.Equal(t, []OwnedCard{
		{Name: "Fire / Ice", Owned: 1},
		{Name: "Island", Owned: 0},
		{Name: "Lightning Bolt", Owned: 0},
		{Name: "Sol Ring", Owned: 3},
	}, owned)
	assert.Equal(t, viewerID, store.countedFor)
	assert.ElementsMatch(t, []string{"sol ring", "island", "fire / ice", "lightning bolt"}, store.countedKeys)
	assert.Equal(t, []deck.Reach{deck.ReachVisible}, store.reaches)
}

func TestGetVisibleDeck_LoadsTheViewersOwnPrivateDeck(t *testing.T) {
	store := compareStore()
	svc := NewService(store, compareUsers())

	visible, err := svc.GetVisibleDeck(context.Background(), viewerID, myDeckID)

	require.NoError(t, err)
	assert.Equal(t, []deck.Reach{deck.ReachVisible}, store.reaches)
	assert.Equal(t, "Mine", visible.Deck.Name)
	assert.Equal(t, viewerID, visible.Owner.ID)
	require.Len(t, visible.Cards, 6)
	assert.True(t, visible.Cards[0].Commander)
	assert.Equal(t, 2, visible.Cards[1].Quantity)
}

func TestGetVisibleDeck_ShowsTheOwnerOfSomeoneElsesSharedDeck(t *testing.T) {
	svc := NewService(compareStore(), compareUsers())

	visible, err := svc.GetVisibleDeck(context.Background(), viewerID, theirDeck)

	require.NoError(t, err)
	assert.Equal(t, "Theirs", visible.Deck.Name)
	assert.Equal(t, "Alice", *visible.Owner.DisplayName)
	assert.Len(t, visible.Cards, 5)
}

func TestVisibleDeckHandler_ReturnsTheSharedDeckShape(t *testing.T) {
	service := &fakeSharedService{shared: SharedDeck{
		Deck:  deck.Deck{ID: myDeckID, Name: "Mine", Visibility: deck.VisibilityPrivate},
		Owner: Owner{ID: viewerID},
		Cards: []Card{{Name: "Island", ScryfallID: "i", Quantity: 3, Board: deck.BoardMain}, {Name: "Opt", Quantity: 1, Board: deck.BoardSideboard}},
	}}
	router := setupProtectedRouter(service, viewerID)

	w := get(router, "/deck/"+myDeckID+"/content")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, viewerID, service.lastUserID)
	assert.Equal(t, myDeckID, service.lastDeckID)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "private", body["deck"].(map[string]any)["visibility"])
	assert.Equal(t, 3.0, body["deck"].(map[string]any)["card_count"])
	assert.Len(t, body["cards"], 2)
	assert.Equal(t, []any{}, body["cards"].([]any)[0].(map[string]any)["tags"])
}

func TestVisibleDeckHandler_Errors(t *testing.T) {
	cases := map[string]struct {
		userID string
		path   string
		err    error
		want   int
	}{
		"unauthenticated": {"", "/deck/" + myDeckID + "/content", nil, http.StatusUnauthorized},
		"invalid id":      {viewerID, "/deck/nope/content", nil, http.StatusNotFound},
		"not visible":     {viewerID, "/deck/" + privateID + "/content", deck.ErrNotFound, http.StatusNotFound},
		"failure":         {viewerID, "/deck/" + myDeckID + "/content", errors.New("db is down"), http.StatusInternalServerError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := setupProtectedRouter(&fakeSharedService{err: tc.err}, tc.userID)

			assert.Equal(t, tc.want, get(router, tc.path).Code)
		})
	}
}
