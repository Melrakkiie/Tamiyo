package deck

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	decks      []Deck
	findAllErr error
}

func (f *fakeRepository) FindAll(ctx context.Context) ([]Deck, error) {
	return f.decks, f.findAllErr
}

func TestService_GetAllDecks_ReturnsDecksFromRepository(t *testing.T) {
	expected := []Deck{
		{ID: 1, Name: "Otterly Playful", Format: "commander"},
		{ID: 2, Name: "Cutelings Everywhere", Format: "commander"},
	}
	repo := &fakeRepository{decks: expected}
	service := NewService(repo)

	result, err := service.GetAllDecks(context.Background())

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetAllDecks_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findAllErr: errors.New("connection lost")}
	service := NewService(repo)

	result, err := service.GetAllDecks(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
}
