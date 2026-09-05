package card

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	cards []Card
	err   error
}

func (f *fakeRepository) FindAll(ctx context.Context) ([]Card, error) {
	return f.cards, f.err
}

func TestService_GetAllCards_ReturnsCardsFromRepository(t *testing.T) {
	expected := []Card{
		{ID: 1, Name: "Black Lotus", SetCode: "lea"},
		{ID: 2, Name: "Lightning Bolt", SetCode: "2xm"},
	}
	repo := &fakeRepository{cards: expected}
	service := NewService(repo)

	result, err := service.GetAllCards(context.Background())

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetAllCards_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{err: errors.New("connection lost")}
	service := NewService(repo)

	result, err := service.GetAllCards(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
}
