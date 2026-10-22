package deckinsights

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/commanderspellbook"
	"Melrakkiie/Tamiyo/internal/deck"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

const otherUserID = "22222222-2222-2222-2222-222222222222"

const testDeckID = "00000000-0000-0000-0000-000000000001"

// fakeDeckReader holds read decks by id. Within ReachOwn it reads only the
// viewer's own decks; other reaches read any deck it holds.
type fakeDeckReader struct {
	decks map[string]deck.ReadDeck
	err   error
}

func (f *fakeDeckReader) ReadDeck(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (deck.ReadDeck, error) {
	if f.err != nil {
		return deck.ReadDeck{}, f.err
	}
	read, ok := f.decks[deckID]
	if !ok || (reach == deck.ReachOwn && read.OwnerID != viewerID) {
		return deck.ReadDeck{}, deck.ErrNotFound
	}
	return read, nil
}

// fakeStore holds stored printings by Scryfall id and the values of every
// deck it is asked about.
type fakeStore struct {
	printings map[string]Printing
	values    DeckValues
	// valuesByOwner, when set, answers only for the owners it holds.
	valuesByOwner map[string]DeckValues
}

func (f fakeStore) DeckValues(ctx context.Context, userID string, deckID string) (DeckValues, error) {
	if f.valuesByOwner != nil {
		return f.valuesByOwner[userID], nil
	}
	return f.values, nil
}

func (f fakeStore) Printings(ctx context.Context, scryfallIDs []string) (map[string]Printing, error) {
	byID := make(map[string]Printing, len(scryfallIDs))
	for _, id := range scryfallIDs {
		if p, ok := f.printings[id]; ok {
			byID[id] = p
		}
	}
	return byID, nil
}

// storeOf is a store holding printings, each keyed by its Scryfall id.
func storeOf(printings ...Printing) fakeStore {
	byID := make(map[string]Printing, len(printings))
	for _, p := range printings {
		byID[p.ScryfallID] = p
	}
	return fakeStore{printings: byID}
}

// recordingFinder stands in for Commander Spellbook: it records the deck it
// is asked about and answers with combos or err.
type recordingFinder struct {
	calls      int
	commanders []string
	main       []string

	combos []commanderspellbook.Combo
	err    error
}

func (r *recordingFinder) FindCombos(ctx context.Context, commanders []string, main []string) ([]commanderspellbook.Combo, error) {
	r.calls++
	r.commanders = commanders
	r.main = main
	return r.combos, r.err
}

func readerWith(read deck.ReadDeck) *fakeDeckReader {
	return &fakeDeckReader{decks: map[string]deck.ReadDeck{read.Deck.ID: read}}
}

// deckOf is the test user's own deck in format, holding entries in the order
// the deck read gives them: commanders first, then by board and name.
func deckOf(format string, entries ...deck.DeckEntry) deck.ReadDeck {
	return deck.ReadDeck{OwnerID: testUserID, Mine: true, Deck: deck.Deck{ID: testDeckID, Format: format}, Entries: entries}
}

func ownedEntry(name string, board string, ownedCopies int) deck.DeckEntry {
	ids := make([]int, ownedCopies)
	for i := range ids {
		ids[i] = i + 1
	}
	return deck.DeckEntry{Name: name, ScryfallID: name + "-id", Board: board, OwnedCardIDs: ids}
}

// owned is a main board entry of p holding one owned copy per card id.
func owned(p Printing, cardIDs ...int) deck.DeckEntry {
	return deck.DeckEntry{Name: p.Name, ScryfallID: p.ScryfallID, Board: deck.BoardMain, OwnedCardIDs: cardIDs}
}

// pending is a main board entry of p holding one pending card of quantity copies.
func pending(p Printing, quantity int) deck.DeckEntry {
	return deck.DeckEntry{Name: p.Name, ScryfallID: p.ScryfallID, Board: deck.BoardMain, Pending: []deck.PendingPart{{ID: 1, Quantity: quantity}}}
}

func onBoard(board string, e deck.DeckEntry) deck.DeckEntry {
	e.Board = board
	return e
}

// gameChanger flags e as the deck read does for a card whose stored printing is a game changer.
func gameChanger(e deck.DeckEntry) deck.DeckEntry {
	e.GameChanger = true
	return e
}

func inSlot(slot deck.CommanderSlot, e deck.DeckEntry) deck.DeckEntry {
	e.Slot = slot
	return e
}

// unstored is a printing the store doesn't hold: not refreshed yet, or a
// placeholder for a printing Scryfall didn't return.
func unstored(name, scryfallID string) Printing {
	return Printing{ScryfallID: scryfallID, Name: name}
}

func TestGetDeckLegality_UnknownDeckReturnsErrNotFound(t *testing.T) {
	svc := NewService(&fakeDeckReader{}, fakeStore{}, nil)

	_, err := svc.GetDeckLegality(context.Background(), testUserID, "00000000-0000-0000-0000-000000000999", deck.ReachOwn)

	require.Error(t, err)
	assert.ErrorIs(t, err, deck.ErrNotFound)
}

func TestGetDeckLegality_PropagatesADeckReadError(t *testing.T) {
	boom := errors.New("db down")
	svc := NewService(&fakeDeckReader{err: boom}, fakeStore{}, nil)

	_, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID, deck.ReachOwn)

	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, deck.ErrNotFound)
}

func TestGetDeckStats_AnotherUsersSharedDeckReturnsErrNotFound(t *testing.T) {
	reader := readerWith(deck.ReadDeck{
		OwnerID: otherUserID,
		Deck:    deck.Deck{ID: testDeckID, Visibility: deck.VisibilityPublic},
		Entries: []deck.DeckEntry{ownedEntry("Island", deck.BoardMain, 1)},
	})
	svc := NewService(reader, fakeStore{}, nil)

	_, err := svc.GetDeckStats(context.Background(), testUserID, testDeckID, deck.ReachOwn)

	assert.ErrorIs(t, err, deck.ErrNotFound)
}

func TestGetDeckStats_CountsOnlyTheMainBoardOfTheOwnersDeck(t *testing.T) {
	reader := readerWith(deck.ReadDeck{
		OwnerID: testUserID,
		Mine:    true,
		Deck:    deck.Deck{ID: testDeckID},
		Entries: []deck.DeckEntry{
			ownedEntry("Island", deck.BoardMain, 3),
			ownedEntry("Negate", deck.BoardSideboard, 2),
			ownedEntry("Opt", deck.BoardConsidering, 1),
		},
	})
	svc := NewService(reader, fakeStore{}, nil)

	stats, err := svc.GetDeckStats(context.Background(), testUserID, testDeckID, deck.ReachOwn)

	require.NoError(t, err)
	assert.Equal(t, 3, stats.CardCount)
}

// sharedDeckOf is a public commander deck of otherUserID holding entries.
func sharedDeckOf(entries ...deck.DeckEntry) deck.ReadDeck {
	return deck.ReadDeck{
		OwnerID: otherUserID,
		Deck:    deck.Deck{ID: testDeckID, Format: "commander", Visibility: deck.VisibilityPublic},
		Entries: entries,
	}
}

func TestGetDeckLegality_SharedReachHidesCardIDs(t *testing.T) {
	channel := legalPrinting(1, "Channel", "Sorcery", nil)
	channel.BannedFormats = []string{"commander"}
	svc := NewService(readerWith(sharedDeckOf(owned(channel, 7))), storeOf(channel), nil)

	report, err := svc.GetDeckLegality(context.Background(), "", testDeckID, deck.ReachShared)

	require.NoError(t, err)
	assert.Equal(t, []LegalityIssue{{CardName: "Channel", Board: deck.BoardMain, Reason: "banned in commander"}}, ignoringDeckSize(report).Issues)
}

func TestGetDeckLegality_OwnReachKeepsCardIDs(t *testing.T) {
	channel := legalPrinting(1, "Channel", "Sorcery", nil)
	channel.BannedFormats = []string{"commander"}
	svc := NewService(readerWith(deckOf("commander", owned(channel, 7))), storeOf(channel), nil)

	report, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID, deck.ReachOwn)

	require.NoError(t, err)
	assert.Equal(t, 7, ignoringDeckSize(report).Issues[0].CardID)
}

func TestGetDeckStats_SharedReachShowsTheOwnersValues(t *testing.T) {
	store := storeOf(solRing)
	store.valuesByOwner = map[string]DeckValues{otherUserID: {
		Owned:       Value{EUR: 14, USD: 18},
		Pending:     Value{EUR: 8, USD: 10},
		Sideboard:   Value{EUR: 9, USD: 11},
		Considering: Value{EUR: 1, USD: 2},
	}}
	svc := NewService(readerWith(sharedDeckOf(owned(solRing, 1))), store, nil)

	for _, viewer := range []string{"", testUserID} {
		stats, err := svc.GetDeckStats(context.Background(), viewer, testDeckID, deck.ReachShared)

		require.NoError(t, err)
		assert.Equal(t, Value{EUR: 22, USD: 28}, stats.Value)
		assert.Equal(t, Value{EUR: 14, USD: 18}, stats.OwnedValue)
		assert.Equal(t, Value{EUR: 8, USD: 10}, stats.PendingValue)
		assert.Equal(t, BoardValues{Sideboard: Value{EUR: 9, USD: 11}, Considering: Value{EUR: 1, USD: 2}}, stats.BoardValues)
	}
}

func TestGetDeckStats_SharedReachCountsGameChangers(t *testing.T) {
	read := sharedDeckOf(inSlot(deck.SlotCommander, gameChanger(owned(kinnan, 1))), gameChanger(owned(rhysticStudy, 2)), owned(solRing, 3))
	svc := NewService(readerWith(read), storeOf(kinnan, rhysticStudy, solRing), nil)

	stats, err := svc.GetDeckStats(context.Background(), "", testDeckID, deck.ReachShared)

	require.NoError(t, err)
	assert.Equal(t, 2, stats.GameChangerCount)
}

func TestGetDeckCombos_SharedReachLooksUpTheSharedDeck(t *testing.T) {
	finder := &recordingFinder{}
	svc := NewService(readerWith(sharedDeckOf(owned(curiosity, 1))), storeOf(curiosity), finder)

	_, err := svc.GetDeckCombos(context.Background(), "", testDeckID, deck.ReachShared)

	require.NoError(t, err)
	assert.Equal(t, []string{"Curiosity"}, finder.main)
}
