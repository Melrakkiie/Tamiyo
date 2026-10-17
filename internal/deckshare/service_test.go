package deckshare

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/deckinsights"
	"Melrakkiie/Tamiyo/internal/user"
)

const ownerID = "11111111-1111-1111-1111-111111111111"
const deckID = "22222222-2222-2222-2222-222222222222"

type fakeDecks struct {
	ownerID   string
	deck      deck.Deck
	sharedErr error
	entries   []deck.DeckEntry
	tags      deck.DeckTags
	tagsErr   error

	lastShareID string
	lastUserID  string
	lastDeckID  string

	publicDecks      []deck.PublicDeck
	lastPublicFilter deck.PublicFilter
}

func (f *fakeDecks) GetSharedDeck(ctx context.Context, deckID string) (string, deck.Deck, error) {
	f.lastShareID = deckID
	if f.sharedErr != nil {
		return "", deck.Deck{}, f.sharedErr
	}
	return f.ownerID, f.deck, nil
}

func (f *fakeDecks) CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error) {
	return map[string]int{}, nil
}

func (f *fakeDecks) BrowsePublicDecks(ctx context.Context, filter deck.PublicFilter) ([]deck.PublicDeck, int, error) {
	f.lastPublicFilter = filter
	return f.publicDecks, len(f.publicDecks), nil
}

func (f *fakeDecks) GetCardTags(ctx context.Context, userID string, deckID string) (deck.DeckTags, error) {
	return f.tags, f.tagsErr
}

func (f *fakeDecks) GetDeck(ctx context.Context, userID string, id string) (deck.Deck, error) {
	return deck.Deck{}, deck.ErrNotFound
}

func (f *fakeDecks) GetDeckContent(ctx context.Context, userID string, deckID string) (deck.DeckContent, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return deck.DeckContent{Deck: f.deck, Entries: f.entries}, nil
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

type fakeInsights struct {
	report     deckinsights.LegalityReport
	stats      deckinsights.DeckStats
	err        error
	lastUserID string
	lastDeckID string
}

func (f *fakeInsights) GetDeckLegality(ctx context.Context, userID string, deckID string) (deckinsights.LegalityReport, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.report, f.err
}

func (f *fakeInsights) GetDeckStats(ctx context.Context, userID string, deckID string) (deckinsights.DeckStats, error) {
	f.lastUserID = userID
	f.lastDeckID = deckID
	return f.stats, f.err
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func newService(decks *fakeDecks, insights *fakeInsights) *Service {
	users := &fakeUsers{users: map[string]user.User{
		ownerID: {ID: ownerID, Email: "alice@example.com", DisplayName: strPtr("Alice"), AvatarScryfallID: strPtr("art")},
	}}
	return NewService(decks, users, insights)
}

func TestService_GetSharedDeck_LoadsCardsAsTheOwner(t *testing.T) {
	decks := &fakeDecks{ownerID: ownerID, deck: deck.Deck{ID: "00000000-0000-0000-0000-000000000009", Name: "Otters"}}
	service := newService(decks, &fakeInsights{})

	shared, err := service.GetSharedDeck(context.Background(), deckID)

	require.NoError(t, err)

	assert.Equal(t, deckID, decks.lastShareID)
	assert.Equal(t, ownerID, decks.lastUserID)
	assert.Equal(t, "00000000-0000-0000-0000-000000000009", decks.lastDeckID)
	assert.Equal(t, "Otters", shared.Deck.Name)
	assert.Equal(t, Owner{ID: ownerID, DisplayName: strPtr("Alice"), AvatarScryfallID: strPtr("art")}, shared.Owner)
}

func TestService_GetSharedDeck_MapsMissingDeckToErrNotFound(t *testing.T) {
	service := newService(&fakeDecks{sharedErr: deck.ErrNotFound}, &fakeInsights{})

	_, err := service.GetSharedDeck(context.Background(), deckID)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_GetSharedDeck_ShowsEachDeckContentEntryAsOneCardSortedByName(t *testing.T) {
	decks := &fakeDecks{
		ownerID: ownerID,
		deck:    deck.Deck{ID: "00000000-0000-0000-0000-000000000009"},
		entries: []deck.DeckEntry{
			{Name: "Tamiyo", ScryfallID: "t", SetCode: "neo", Board: deck.BoardMain, Slot: deck.SlotCommander, Pending: []deck.PendingPart{{ID: 8, Quantity: 1}}},
			{Name: "Island", ScryfallID: "i", SetCode: "neo", Board: deck.BoardMain, OwnedCardIDs: []int{3}, Pending: []deck.PendingPart{{ID: 5, Quantity: 2}}},
			{Name: "Island", ScryfallID: "i", SetCode: "neo", Board: deck.BoardMain, Proxy: true, OwnedCardIDs: []int{2}},
			{Name: "Tamiyo", ScryfallID: "t", SetCode: "neo", Board: deck.BoardMain, OwnedCardIDs: []int{1}},
			{Name: "Brainstorm", ScryfallID: "b", SetCode: "ice", Board: deck.BoardSideboard, Foil: true, Pending: []deck.PendingPart{{ID: 6, Quantity: 4}}, ManaValue: 1, Colors: strPtr("U"), CardType: strPtr("Instant"), ColorIdentity: strPtr("U")},
		},
	}
	service := newService(decks, &fakeInsights{})

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
	decks := &fakeDecks{
		ownerID: ownerID,
		deck:    deck.Deck{ID: "00000000-0000-0000-0000-000000000009"},
		entries: []deck.DeckEntry{
			{Name: "Island", ScryfallID: "i", SetCode: "neo", OwnedCardIDs: []int{1}},
			{Name: "Island", ScryfallID: "i", SetCode: "neo", Foil: true, OwnedCardIDs: []int{2}},
			{Name: "Fire // Ice", ScryfallID: "f", SetCode: "mh2", OwnedCardIDs: []int{3}},
			{Name: "Brainstorm", ScryfallID: "b", SetCode: "ice", Pending: []deck.PendingPart{{ID: 6, Quantity: 1}}},
		},
		tags: deck.DeckTags{Cards: []deck.TaggedCard{
			{Name: "Island", Tags: []string{"Terrain"}},
			{Name: "Fire / Ice", Tags: []string{"Removal", "Pioche"}},
		}},
	}
	service := newService(decks, &fakeInsights{})

	shared, err := service.GetSharedDeck(context.Background(), deckID)

	require.NoError(t, err)
	require.Len(t, shared.Cards, 4)
	assert.Nil(t, shared.Cards[0].Tags)
	assert.Equal(t, []string{"Removal", "Pioche"}, shared.Cards[1].Tags)
	assert.Equal(t, []string{"Terrain"}, shared.Cards[2].Tags)
	assert.Equal(t, []string{"Terrain"}, shared.Cards[3].Tags)
}

func TestService_GetSharedDeck_PropagatesTagErrors(t *testing.T) {
	decks := &fakeDecks{ownerID: ownerID, deck: deck.Deck{ID: deckID}, tagsErr: errors.New("db is down")}
	service := newService(decks, &fakeInsights{})

	_, err := service.GetSharedDeck(context.Background(), deckID)

	require.Error(t, err)
}

func TestService_GetSharedDeckLegality_ChecksAsTheOwnerAndHidesCardIDs(t *testing.T) {
	insights := &fakeInsights{report: deckinsights.LegalityReport{
		Format: "commander",
		Issues: []deckinsights.LegalityIssue{{CardID: 12, CardName: "Black Lotus", Reason: "banned in commander"}},
	}}
	service := newService(&fakeDecks{ownerID: ownerID, deck: deck.Deck{ID: "00000000-0000-0000-0000-000000000009"}}, insights)

	report, err := service.GetSharedDeckLegality(context.Background(), deckID)

	require.NoError(t, err)
	assert.Equal(t, ownerID, insights.lastUserID)
	assert.Equal(t, "00000000-0000-0000-0000-000000000009", insights.lastDeckID)
	assert.Equal(t, []deckinsights.LegalityIssue{{CardName: "Black Lotus", Reason: "banned in commander"}}, report.Issues)
}

func TestService_GetSharedDeckStats_ComputesAsTheOwner(t *testing.T) {
	insights := &fakeInsights{stats: deckinsights.DeckStats{CardCount: 100}}
	service := newService(&fakeDecks{ownerID: ownerID, deck: deck.Deck{ID: "00000000-0000-0000-0000-000000000009"}}, insights)

	stats, err := service.GetSharedDeckStats(context.Background(), deckID)

	require.NoError(t, err)
	assert.Equal(t, 100, stats.CardCount)
	assert.Equal(t, ownerID, insights.lastUserID)
}

func TestService_GetSharedDeckStats_DoesNotComputeForAHiddenDeck(t *testing.T) {
	insights := &fakeInsights{}
	service := newService(&fakeDecks{sharedErr: deck.ErrNotFound}, insights)

	_, err := service.GetSharedDeckStats(context.Background(), deckID)

	assert.ErrorIs(t, err, ErrNotFound)
	assert.Empty(t, insights.lastUserID)
}

func TestService_GetSharedDeckStats_PropagatesInsightErrors(t *testing.T) {
	boom := errors.New("boom")
	service := newService(&fakeDecks{ownerID: ownerID, deck: deck.Deck{ID: "00000000-0000-0000-0000-000000000009"}}, &fakeInsights{err: boom})

	_, err := service.GetSharedDeckStats(context.Background(), deckID)

	assert.ErrorIs(t, err, boom)
}

func TestService_BrowsePublicDecks_PassesTheFilter(t *testing.T) {
	decks := &fakeDecks{publicDecks: []deck.PublicDeck{{ID: deckID, Name: "Otters"}}}
	service := newService(decks, &fakeInsights{})

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
