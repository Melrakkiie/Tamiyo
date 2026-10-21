package deckinsights

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"Melrakkiie/Tamiyo/internal/deck"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

const otherUserID = "22222222-2222-2222-2222-222222222222"

const testDeckID = "00000000-0000-0000-0000-000000000001"

// --- fakes -------------------------------------------------------------

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

type fakeStore struct{}

func (fakeStore) DeckValues(ctx context.Context, userID string, deckID string) (DeckValues, error) {
	return DeckValues{}, nil
}

func (fakeStore) Printings(ctx context.Context, scryfallIDs []string) (map[string]Printing, error) {
	return map[string]Printing{}, nil
}

func readerWith(read deck.ReadDeck) *fakeDeckReader {
	return &fakeDeckReader{decks: map[string]deck.ReadDeck{read.Deck.ID: read}}
}

func ownedEntry(name string, board string, ownedCopies int) deck.DeckEntry {
	ids := make([]int, ownedCopies)
	for i := range ids {
		ids[i] = i + 1
	}
	return deck.DeckEntry{Name: name, ScryfallID: name + "-id", Board: board, OwnedCardIDs: ids}
}

// --- GetDeckLegality ------------------------------------------------------

func TestGetDeckLegality_UnknownDeckReturnsErrNotFound(t *testing.T) {
	svc := NewService(&fakeDeckReader{}, fakeStore{}, nil)

	_, err := svc.GetDeckLegality(context.Background(), testUserID, "00000000-0000-0000-0000-000000000999")

	require.Error(t, err)
	assert.ErrorIs(t, err, deck.ErrNotFound)
}

func TestGetDeckLegality_PropagatesADeckReadError(t *testing.T) {
	boom := errors.New("db down")
	svc := NewService(&fakeDeckReader{err: boom}, fakeStore{}, nil)

	_, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)

	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, deck.ErrNotFound)
}

// --- GetDeckStats -----------------------------------------------------------

func TestGetDeckStats_AnotherUsersSharedDeckReturnsErrNotFound(t *testing.T) {
	reader := readerWith(deck.ReadDeck{
		OwnerID: otherUserID,
		Deck:    deck.Deck{ID: testDeckID, Visibility: deck.VisibilityPublic},
		Entries: []deck.DeckEntry{ownedEntry("Island", deck.BoardMain, 1)},
	})
	svc := NewService(reader, fakeStore{}, nil)

	_, err := svc.GetDeckStats(context.Background(), testUserID, testDeckID)

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

	stats, err := svc.GetDeckStats(context.Background(), testUserID, testDeckID)

	require.NoError(t, err)
	assert.Equal(t, 3, stats.CardCount)
}
