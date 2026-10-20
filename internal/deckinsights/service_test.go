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

const testDeckID = "00000000-0000-0000-0000-000000000001"

// --- fakes -------------------------------------------------------------

type fakeDeckService struct {
	contents map[string]deck.DeckContent
	err      error
}

func (f *fakeDeckService) GetDeckContent(ctx context.Context, userID string, deckID string) (deck.DeckContent, error) {
	if f.err != nil {
		return deck.DeckContent{}, f.err
	}
	content, ok := f.contents[deckID]
	if !ok {
		return deck.DeckContent{}, deck.ErrNotFound
	}
	return content, nil
}

type fakeStore struct{}

func (fakeStore) DeckValues(ctx context.Context, userID string, deckID string) (DeckValues, error) {
	return DeckValues{}, nil
}

func (fakeStore) Printings(ctx context.Context, scryfallIDs []string) (map[string]Printing, error) {
	return map[string]Printing{}, nil
}

// --- GetDeckLegality ------------------------------------------------------

func TestGetDeckLegality_UnknownDeckReturnsErrDeckNotFound(t *testing.T) {
	svc := NewService(&fakeDeckService{}, fakeStore{})

	_, err := svc.GetDeckLegality(context.Background(), testUserID, "00000000-0000-0000-0000-000000000999")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeckNotFound)
}

func TestGetDeckLegality_PropagatesADeckContentError(t *testing.T) {
	boom := errors.New("db down")
	svc := NewService(&fakeDeckService{err: boom}, fakeStore{})

	_, err := svc.GetDeckLegality(context.Background(), testUserID, testDeckID)

	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrDeckNotFound)
}
