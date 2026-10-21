package deckshare

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/user"
)

const ownerID = "11111111-1111-1111-1111-111111111111"
const deckID = "22222222-2222-2222-2222-222222222222"

type fakeDecks struct {
	read    deck.ReadDeck
	readErr error

	lastReadID string
	lastReach  deck.Reach

	publicDecks      []deck.PublicDeck
	lastPublicFilter deck.PublicFilter
}

func (f *fakeDecks) ReadDeck(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (deck.ReadDeck, error) {
	f.lastReadID = deckID
	f.lastReach = reach
	if f.readErr != nil {
		return deck.ReadDeck{}, f.readErr
	}
	return f.read, nil
}

func (f *fakeDecks) CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error) {
	return map[string]int{}, nil
}

func (f *fakeDecks) BrowsePublicDecks(ctx context.Context, filter deck.PublicFilter) ([]deck.PublicDeck, int, error) {
	f.lastPublicFilter = filter
	return f.publicDecks, len(f.publicDecks), nil
}

type fakeUsers struct {
	users map[string]user.User
}

func (f *fakeUsers) GetUser(ctx context.Context, userID string) (user.User, error) {
	u, ok := f.users[userID]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return u, nil
}

func sharedRead() deck.ReadDeck {
	return deck.ReadDeck{OwnerID: ownerID, Deck: deck.Deck{ID: deckID, Name: "Otters"}}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func newService(decks *fakeDecks) *Service {
	users := &fakeUsers{users: map[string]user.User{
		ownerID: {ID: ownerID, Email: "alice@example.com", DisplayName: strPtr("Alice"), AvatarScryfallID: strPtr("art")},
	}}
	return NewService(decks, users)
}

func TestService_GetSharedDeck_ReadsTheSharedDeckWithItsOwnersProfile(t *testing.T) {
	decks := &fakeDecks{read: sharedRead()}
	service := newService(decks)

	shared, err := service.GetSharedDeck(context.Background(), deckID)

	require.NoError(t, err)
	assert.Equal(t, deck.ReachShared, decks.lastReach)
	assert.Equal(t, deckID, decks.lastReadID)
	assert.Equal(t, "Otters", shared.Deck.Name)
	assert.Equal(t, Owner{ID: ownerID, DisplayName: strPtr("Alice"), AvatarScryfallID: strPtr("art")}, shared.Owner)
}

func TestService_GetSharedDeck_ShowsEachDeckContentEntryAsOneCardSortedByName(t *testing.T) {
	read := sharedRead()
	read.Entries = []deck.DeckEntry{
		{Name: "Tamiyo", ScryfallID: "t", SetCode: "neo", Board: deck.BoardMain, Slot: deck.SlotCommander, Pending: []deck.PendingPart{{ID: 8, Quantity: 1}}},
		{Name: "Island", ScryfallID: "i", SetCode: "neo", Board: deck.BoardMain, OwnedCardIDs: []int{3}, Pending: []deck.PendingPart{{ID: 5, Quantity: 2}}},
		{Name: "Island", ScryfallID: "i", SetCode: "neo", Board: deck.BoardMain, Proxy: true, OwnedCardIDs: []int{2}},
		{Name: "Tamiyo", ScryfallID: "t", SetCode: "neo", Board: deck.BoardMain, OwnedCardIDs: []int{1}},
		{Name: "Brainstorm", ScryfallID: "b", SetCode: "ice", Board: deck.BoardSideboard, Foil: true, Pending: []deck.PendingPart{{ID: 6, Quantity: 4}}, ManaValue: 1, Colors: strPtr("U"), CardType: strPtr("Instant"), ColorIdentity: strPtr("U")},
	}
	service := newService(&fakeDecks{read: read})

	shared, err := service.GetSharedDeck(context.Background(), deckID)

	require.NoError(t, err)
	assert.Equal(t, []Card{
		{Name: "Brainstorm", ScryfallID: "b", SetCode: "ice", Board: deck.BoardSideboard, Foil: true, Quantity: 4, ManaValue: 1, Colors: strPtr("U"), CardType: strPtr("Instant"), ColorIdentity: strPtr("U")},
		{Name: "Island", ScryfallID: "i", SetCode: "neo", Board: deck.BoardMain, Quantity: 3},
		{Name: "Island", ScryfallID: "i", SetCode: "neo", Board: deck.BoardMain, Quantity: 1},
		{Name: "Tamiyo", ScryfallID: "t", SetCode: "neo", Board: deck.BoardMain, Quantity: 1, Commander: true},
		{Name: "Tamiyo", ScryfallID: "t", SetCode: "neo", Board: deck.BoardMain, Quantity: 1},
	}, shared.Cards)
}

func TestService_GetSharedDeck_AddsEachCardsTagsWhateverThePrinting(t *testing.T) {
	read := sharedRead()
	read.Entries = []deck.DeckEntry{
		{Name: "Island", ScryfallID: "i", SetCode: "neo", OwnedCardIDs: []int{1}},
		{Name: "Island", ScryfallID: "i", SetCode: "neo", Foil: true, OwnedCardIDs: []int{2}},
		{Name: "Fire // Ice", ScryfallID: "f", SetCode: "mh2", OwnedCardIDs: []int{3}},
		{Name: "Brainstorm", ScryfallID: "b", SetCode: "ice", Pending: []deck.PendingPart{{ID: 6, Quantity: 1}}},
	}
	read.Tags = map[string][]string{
		"island":     {"Terrain"},
		"fire / ice": {"Removal", "Pioche"},
	}
	service := newService(&fakeDecks{read: read})

	shared, err := service.GetSharedDeck(context.Background(), deckID)

	require.NoError(t, err)
	require.Len(t, shared.Cards, 4)
	assert.Nil(t, shared.Cards[0].Tags)
	assert.Equal(t, []string{"Removal", "Pioche"}, shared.Cards[1].Tags)
	assert.Equal(t, []string{"Terrain"}, shared.Cards[2].Tags)
	assert.Equal(t, []string{"Terrain"}, shared.Cards[3].Tags)
}

func TestService_GetSharedDeck_PropagatesReadErrors(t *testing.T) {
	boom := errors.New("db is down")
	service := newService(&fakeDecks{readErr: boom})

	_, err := service.GetSharedDeck(context.Background(), deckID)

	assert.ErrorIs(t, err, boom)
}

func TestService_BrowsePublicDecks_PassesTheFilter(t *testing.T) {
	decks := &fakeDecks{publicDecks: []deck.PublicDeck{{ID: deckID, Name: "Otters"}}}
	service := newService(decks)

	found, total, err := service.BrowsePublicDecks(context.Background(), deck.PublicFilter{Format: "commander"})

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "Otters", found[0].Name)
	assert.Equal(t, "commander", decks.lastPublicFilter.Format)
}

func (f *fakeDecks) LikeDeck(ctx context.Context, userID string, deckID string) error { return nil }

func (f *fakeDecks) UnlikeDeck(ctx context.Context, userID string, deckID string) error { return nil }

func (f *fakeDecks) GetLikeStatus(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error) {
	return deck.LikeStatus{}, nil
}

func (f *fakeDecks) GetLikedDecks(ctx context.Context, userID string, page int, limit int) ([]deck.PublicDeck, int, error) {
	return nil, 0, nil
}
