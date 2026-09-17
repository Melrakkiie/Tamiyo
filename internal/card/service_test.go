package card

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	cards      []Card
	findAllErr error
	storageID  *int

	findByIDCard Card
	findByIDErr  error

	createdCard Card
	createErr   error

	deleteErr error
}

func (f *fakeRepository) FindAll(ctx context.Context, storageID *int) ([]Card, error) {
	f.storageID = storageID
	return f.cards, f.findAllErr
}

func (f *fakeRepository) FindByID(ctx context.Context, id int) (Card, error) {
	if f.findByIDErr != nil {
		return Card{}, f.findByIDErr
	}
	return f.findByIDCard, nil
}

func (f *fakeRepository) Create(ctx context.Context, c Card) (Card, error) {
	if f.createErr != nil {
		return Card{}, f.createErr
	}
	f.createdCard = c
	c.ID = 1
	return c, nil
}

func (f *fakeRepository) Delete(ctx context.Context, id int) error {
	return f.deleteErr
}

func TestService_GetAllCards_ReturnsCardsFromRepository(t *testing.T) {
	expected := []Card{
		{ID: 1, Name: "Black Lotus", SetCode: "lea"},
		{ID: 2, Name: "Lightning Bolt", SetCode: "2xm"},
	}
	repo := &fakeRepository{cards: expected}
	service := NewService(repo)

	result, err := service.GetAllCards(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetAllCards_PassesStorageIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	testID := 1
	_, err := service.GetAllCards(context.Background(), &testID)

	require.NoError(t, err)
	require.NotNil(t, repo.storageID)
	assert.Equal(t, testID, *repo.storageID)
}

func TestService_GetAllCards_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findAllErr: errors.New("connection lost")}
	service := NewService(repo)

	result, err := service.GetAllCards(context.Background(), nil)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestService_GetCard_ReturnsCardFromRepository(t *testing.T) {
	expected := Card{ID: 1, Name: "Black Lotus", SetCode: "lea"}
	repo := &fakeRepository{findByIDCard: expected}
	service := NewService(repo)

	result, err := service.GetCard(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetCard_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetCard(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_CreateCard_PassesCardUnchangedToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	storageID := 2
	input := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: 322,
		Foil:            false,
		StorageID:       &storageID,
	}

	_, err := service.CreateCard(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, input, repo.createdCard)
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

func TestService_DeleteCard_PropagatesRepositorySuccess(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	err := service.DeleteCard(context.Background(), 1)

	assert.NoError(t, err)
}

func TestService_DeleteCard_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{deleteErr: ErrNotFound}
	service := NewService(repo)

	err := service.DeleteCard(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_DeleteCard_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{deleteErr: errors.New("delete failed")}
	service := NewService(repo)

	err := service.DeleteCard(context.Background(), 1)

	assert.Error(t, err)
}
