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

	findByIDDeck Deck
	findByIDErr  error

	createdDeck Deck
	createErr   error
}

func (f *fakeRepository) FindAll(ctx context.Context) ([]Deck, error) {
	return f.decks, f.findAllErr
}

func (f *fakeRepository) FindByID(ctx context.Context, id int) (Deck, error) {
	if f.findByIDErr != nil {
		return Deck{}, f.findByIDErr
	}
	return f.findByIDDeck, nil
}

func (f *fakeRepository) Create(ctx context.Context, d Deck) (Deck, error) {
	if f.createErr != nil {
		return Deck{}, f.createErr
	}
	f.createdDeck = d
	d.ID = 1
	return d, nil
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

func TestService_GetDeck_ReturnsDeckFromRepository(t *testing.T) {
	expected := Deck{ID: 1, Name: "Otterly Playful", Format: "commander"}
	repo := &fakeRepository{findByIDDeck: expected}
	service := NewService(repo)

	result, err := service.GetDeck(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetDeck_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetDeck(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_CreateDeck_PassesDeckUnchangedToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	input := Deck{
		Name:   "Otterly Playful",
		Format: "commander",
	}

	_, err := service.CreateDeck(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, input, repo.createdDeck)
}

func TestService_CreateDeck_ReturnsDeckFromRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	result, err := service.CreateDeck(context.Background(), Deck{Name: "Otterly Playful", Format: "commander"})

	require.NoError(t, err)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, "Otterly Playful", result.Name)
}

func TestService_CreateDeck_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{createErr: errors.New("insert failed")}
	service := NewService(repo)

	result, err := service.CreateDeck(context.Background(), Deck{Name: "Otterly Playful"})

	assert.Error(t, err)
	assert.Equal(t, Deck{}, result)
}
