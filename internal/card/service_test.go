package card

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	cards      []Card
	findAllErr error

	createdCard Card
	createErr   error
}

func (f *fakeRepository) FindAll(ctx context.Context) ([]Card, error) {
	return f.cards, f.findAllErr
}

func (f *fakeRepository) Create(ctx context.Context, c Card) (Card, error) {
	if f.createErr != nil {
		return Card{}, f.createErr
	}
	f.createdCard = c
	c.ID = 1
	return c, nil
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
	repo := &fakeRepository{findAllErr: errors.New("connection lost")}
	service := NewService(repo)

	result, err := service.GetAllCards(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestService_CreateCard_SetsAddedToCurrentTime(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	before := time.Now()
	_, err := service.CreateCard(context.Background(), Card{Name: "Sol Ring"})
	after := time.Now()

	require.NoError(t, err)
	assert.False(t, repo.createdCard.Added.Before(before), "Added devrait être postérieur ou égal à 'before'")
	assert.False(t, repo.createdCard.Added.After(after), "Added devrait être antérieur ou égal à 'after'")
}

func TestService_CreateCard_ReturnsCardFromRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	result, err := service.CreateCard(context.Background(), Card{Name: "Sol Ring", SetCode: "cmr"})

	require.NoError(t, err)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, "Sol Ring", result.Name)
}

func TestService_CreateCard_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{createErr: errors.New("insert failed")}
	service := NewService(repo)

	result, err := service.CreateCard(context.Background(), Card{Name: "Sol Ring"})

	assert.Error(t, err)
	assert.Equal(t, Card{}, result)
}
