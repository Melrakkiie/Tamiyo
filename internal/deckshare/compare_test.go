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

type storedDeck struct {
	ownerID string
	deck    deck.Deck
	shared  bool
	cards   []deck.DeckCard
	pending []deck.PendingCard
	tags    deck.DeckTags
}

type fakeDeckStore struct {
	decks   map[string]storedDeck
	getErr  error
	visited []string

	owned       map[string]int
	countedFor  string
	countedKeys []string
}

func (f *fakeDeckStore) GetDeck(ctx context.Context, userID string, id string) (deck.Deck, error) {
	if f.getErr != nil {
		return deck.Deck{}, f.getErr
	}
	d, ok := f.decks[id]
	if !ok || d.ownerID != userID {
		return deck.Deck{}, deck.ErrNotFound
	}
	return d.deck, nil
}

func (f *fakeDeckStore) GetSharedDeck(ctx context.Context, id string) (string, deck.Deck, error) {
	d, ok := f.decks[id]
	if !ok || !d.shared {
		return "", deck.Deck{}, deck.ErrNotFound
	}
	return d.ownerID, d.deck, nil
}

func (f *fakeDeckStore) GetDeckCards(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]deck.DeckCard, error) {
	f.visited = append(f.visited, userID+":"+id)
	return f.decks[id].cards, nil
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

func (f *fakeDeckStore) GetCardTags(ctx context.Context, userID string, deckID string) (deck.DeckTags, error) {
	return f.decks[deckID].tags, nil
}

func (f *fakeDeckStore) GetPendingCards(ctx context.Context, userID string, deckID string) ([]deck.PendingCard, error) {
	return f.decks[deckID].pending, nil
}

func compareStore() *fakeDeckStore {
	return &fakeDeckStore{decks: map[string]storedDeck{
		myDeckID: {
			ownerID: viewerID,
			deck:    deck.Deck{ID: myDeckID, Name: "Mine", Format: "commander", Visibility: deck.VisibilityPrivate, CommanderID: intPtr(1)},
			cards: []deck.DeckCard{
				{ID: 1, Name: "Atraxa, Praetors' Voice", ScryfallID: "atraxa-cmr", ManaValue: 4},
				{ID: 2, Name: "Sol Ring", ScryfallID: "sol-sld"},
				{ID: 3, Name: "Sol Ring", ScryfallID: "sol-cmm"},
				{ID: 4, Name: "Island", ScryfallID: "island-a"},
				{ID: 5, Name: "Fire // Ice", ScryfallID: "fire-mh2", CardType: strPtr("Instant"), Colors: strPtr("UR"), ColorIdentity: strPtr("UR")},
			},
			pending: []deck.PendingCard{{ID: 9, Name: "Counterspell", ScryfallID: "cs", Quantity: 2}},
			tags:    deck.DeckTags{Cards: []deck.TaggedCard{{Name: "Sol Ring", Tags: []string{"Ramp"}}}},
		},
		theirDeck: {
			ownerID: strangerID,
			shared:  true,
			deck:    deck.Deck{ID: theirDeck, Name: "Theirs", Format: "commander", Visibility: deck.VisibilityUnlisted},
			cards: []deck.DeckCard{
				{ID: 20, Name: "Sol Ring", ScryfallID: "sol-lea"},
				{ID: 21, Name: "Island", ScryfallID: "island-b"},
				{ID: 22, Name: "Island", ScryfallID: "island-c"},
				{ID: 23, Name: "Fire / Ice", ScryfallID: "fire-other"},
				{ID: 24, Name: "Lightning Bolt", ScryfallID: "bolt"},
			},
			tags: deck.DeckTags{Cards: []deck.TaggedCard{
				{Name: "Sol Ring", Tags: []string{"Mana"}},
				{Name: "Fire / Ice", Tags: []string{"Removal"}},
			}},
		},
		privateID: {
			ownerID: strangerID,
			deck:    deck.Deck{ID: privateID, Name: "Hidden", Visibility: deck.VisibilityPrivate},
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
	svc := NewService(store, compareUsers(), &fakeInsights{})

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
	assert.Equal(t, []string{"Ramp"}, solRing.Tags)
	assert.Equal(t, []string{"Mana"}, solRing.OtherTags)
	assert.Nil(t, comparison.Common[0].Tags)
	assert.Equal(t, []string{"Removal"}, comparison.Common[0].OtherTags)
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
	svc := NewService(store, compareUsers(), &fakeInsights{})

	comparison, err := svc.CompareDecks(context.Background(), viewerID, myDeckID, theirDeck)

	require.NoError(t, err)
	assert.True(t, comparison.Deck.Mine)
	assert.Equal(t, 7, comparison.Deck.CardCount)
	assert.False(t, comparison.Other.Mine)
	assert.Equal(t, "Alice", *comparison.Other.Owner.DisplayName)
	assert.Equal(t, 5, comparison.Other.CardCount)
	assert.Contains(t, store.visited, strangerID+":"+theirDeck)
}

func TestCompareDecks_TwoDecksSharedByOthers(t *testing.T) {
	svc := NewService(compareStore(), compareUsers(), &fakeInsights{})

	comparison, err := svc.CompareDecks(context.Background(), viewerID, theirDeck, theirDeck)

	require.NoError(t, err)
	assert.False(t, comparison.Deck.Mine)
	assert.Len(t, comparison.Common, 4)
	assert.Empty(t, comparison.OnlyInDeck)
	assert.Empty(t, comparison.OnlyInOther)
}

func TestCompareDecks_SomeoneElsesPrivateDeckIsNotFound(t *testing.T) {
	svc := NewService(compareStore(), compareUsers(), &fakeInsights{})

	_, err := svc.CompareDecks(context.Background(), viewerID, myDeckID, privateID)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = svc.CompareDecks(context.Background(), viewerID, "88888888-8888-8888-8888-888888888888", myDeckID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCompareDecks_WithoutAViewerOnlySeesSharedDecks(t *testing.T) {
	svc := NewService(compareStore(), compareUsers(), &fakeInsights{})

	comparison, err := svc.CompareDecks(context.Background(), "", theirDeck, theirDeck)
	require.NoError(t, err)
	assert.False(t, comparison.Deck.Mine)
	assert.False(t, comparison.Other.Mine)
	assert.Len(t, comparison.Common, 4)

	_, err = svc.CompareDecks(context.Background(), "", theirDeck, myDeckID)
	assert.ErrorIs(t, err, ErrNotFound)
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
	assert.Equal(t, http.StatusNotFound, get(setupRouter(&fakeSharedService{err: ErrNotFound}), "/shared/decks/"+theirDeck+"/compare/"+privateID).Code)
}

func TestCompareDecks_PropagatesUnexpectedErrors(t *testing.T) {
	store := compareStore()
	store.getErr = errors.New("db is down")
	svc := NewService(store, compareUsers(), &fakeInsights{})

	_, err := svc.CompareDecks(context.Background(), viewerID, myDeckID, theirDeck)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
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
	assert.Equal(t, []any{}, common["tags"])
	assert.Equal(t, []any{}, common["other_tags"])
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
		"not visible":     {viewerID, "/deck/" + myDeckID + "/compare/" + privateID, ErrNotFound, http.StatusNotFound},
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
	mine := store.decks[myDeckID]
	mine.cards = append(mine.cards, deck.DeckCard{ID: 6, Name: "Lightning Bolt", ScryfallID: "bolt", Board: deck.BoardSideboard})
	mine.pending = append(mine.pending, deck.PendingCard{ID: 10, Name: "Opt", ScryfallID: "opt", Quantity: 1, Board: deck.BoardConsidering})
	store.decks[myDeckID] = mine
	svc := NewService(store, compareUsers(), &fakeInsights{})

	comparison, err := svc.CompareDecks(context.Background(), viewerID, myDeckID, theirDeck)

	require.NoError(t, err)
	assert.Equal(t, []string{"Lightning Bolt"}, names(comparison.OnlyInOther))
	assert.Equal(t, []string{"Atraxa, Praetors' Voice", "Counterspell"}, names(comparison.OnlyInDeck))
	assert.Equal(t, 7, comparison.Deck.CardCount)
}

func TestCollectionOwnership_CountsTheViewersCopiesByName(t *testing.T) {
	store := compareStore()
	store.owned = map[string]int{"sol ring": 3, "fire / ice": 1}
	svc := NewService(store, compareUsers(), &fakeInsights{})

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
}

func TestCollectionOwnership_WorksOnTheViewersOwnDeckButNotOnAPrivateOne(t *testing.T) {
	svc := NewService(compareStore(), compareUsers(), &fakeInsights{})

	owned, err := svc.CollectionOwnership(context.Background(), viewerID, myDeckID)
	require.NoError(t, err)
	assert.Len(t, owned, 5)

	_, err = svc.CollectionOwnership(context.Background(), viewerID, privateID)
	assert.ErrorIs(t, err, ErrNotFound)
}
