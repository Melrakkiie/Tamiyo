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

	updatedDeck Deck
	updateErr   error

	deleteErr error

	getDeckCards    []DeckCard
	getDeckCardsErr error

	linkErr error

	unlinkErr error
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

func (f *fakeRepository) Update(ctx context.Context, d Deck) (Deck, error) {
	if f.updateErr != nil {
		return Deck{}, f.updateErr
	}
	f.updatedDeck = d
	return d, nil
}

func (f *fakeRepository) Delete(ctx context.Context, id int) error {
	return f.deleteErr
}

func (f *fakeRepository) FindCardsByDeckID(ctx context.Context, id int) ([]DeckCard, error) {
	if f.getDeckCardsErr != nil {
		return nil, f.getDeckCardsErr
	}
	return f.getDeckCards, nil
}

func (f *fakeRepository) LinkCardToDeck(ctx context.Context, deckID, cardID int) error {
	return f.linkErr
}

func (f *fakeRepository) UnlinkCardFromDeck(ctx context.Context, deckID, cardID int) error {
	return f.unlinkErr
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

func TestService_UpdateDeck_AppliesPartialChangesOnExistingDeck(t *testing.T) {
	existing := Deck{ID: 1, Name: "Red Deck", Format: "modern", CommanderID: nil}
	repo := &fakeRepository{findByIDDeck: existing}
	service := NewService(repo)

	newName := "Renamed"
	req := updateDeckRequest{Name: &newName}

	result, err := service.UpdateDeck(context.Background(), 1, req)

	require.NoError(t, err)
	assert.Equal(t, "Renamed", result.Name)
	assert.Equal(t, "modern", result.Format)
}

func TestService_UpdateDeck_ReturnsNotFoundWhenDeckDoesNotExist(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	newName := "Doesn't matter"
	req := updateDeckRequest{Name: &newName}

	_, err := service.UpdateDeck(context.Background(), 999, req)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_UpdateDeck_PropagatesRepositoryUpdateError(t *testing.T) {
	existing := Deck{ID: 1, Name: "Red Deck", Format: "modern", CommanderID: nil}
	repo := &fakeRepository{findByIDDeck: existing, updateErr: errors.New("update failed")}
	service := NewService(repo)

	newName := "New Name"
	req := updateDeckRequest{Name: &newName}

	_, err := service.UpdateDeck(context.Background(), 1, req)

	assert.Error(t, err)
}

func TestService_DeleteDeck_PropagatesRepositorySuccess(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	err := service.DeleteDeck(context.Background(), 1)

	assert.NoError(t, err)
}

func TestService_DeleteDeck_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{deleteErr: ErrNotFound}
	service := NewService(repo)

	err := service.DeleteDeck(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_DeleteDeck_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{deleteErr: errors.New("delete failed")}
	service := NewService(repo)

	err := service.DeleteDeck(context.Background(), 1)

	assert.Error(t, err)
}

func TestService_GetDeckCards_ReturnsDecksFromRepository(t *testing.T) {
	expected := []DeckCard{
		{ID: 1, Name: "Black Lotus", SetCode: "lea"},
		{ID: 2, Name: "Lightning Bolt", SetCode: "2xm"},
	}
	repo := &fakeRepository{getDeckCards: expected}
	service := NewService(repo)

	result, err := service.GetDeckCards(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetDeckCards_ReturnsNotFoundWhenDeckDoesNotExist(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetDeckCards(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_GetDeckCards_PropagatesRepositoryUpdateError(t *testing.T) {
	repo := &fakeRepository{getDeckCardsErr: errors.New("connection lost")}
	service := NewService(repo)

	result, err := service.GetDeckCards(context.Background(), 1)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestService_PutCardInDeck_PropagatesRepositorySuccess(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	err := service.PutCardInDeck(context.Background(), 1, 4)

	assert.NoError(t, err)
}

func TestService_PutCardInDeck_PropagatesDeckNotFoundError(t *testing.T) {
	repo := &fakeRepository{linkErr: ErrNotFound}
	service := NewService(repo)

	err := service.PutCardInDeck(context.Background(), 999, 4)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_PutCardInDeck_PropagatesCardNotFoundError(t *testing.T) {
	repo := &fakeRepository{linkErr: ErrCardNotFound}
	service := NewService(repo)

	err := service.PutCardInDeck(context.Background(), 1, 9999)

	assert.ErrorIs(t, err, ErrCardNotFound)
}

func TestService_PutCardInDeck_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{linkErr: errors.New("insert failed")}
	service := NewService(repo)

	err := service.PutCardInDeck(context.Background(), 1, 4)

	assert.Error(t, err)
}

func TestService_RemoveCardFromDeck_PropagatesRepositorySuccess(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	err := service.RemoveCardFromDeck(context.Background(), 1, 4)

	assert.NoError(t, err)
}

func TestService_RemoveCardFromDeck_SucceedsWhenLinkDoesNotExist(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	err := service.RemoveCardFromDeck(context.Background(), 1, 999)

	assert.NoError(t, err)
}

func TestService_RemoveCardFromDeck_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{unlinkErr: errors.New("delete failed")}
	service := NewService(repo)

	err := service.RemoveCardFromDeck(context.Background(), 1, 4)

	assert.Error(t, err)
}
