package card

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testUserID = "11111111-1111-1111-1111-111111111111"
const otherUserID = "22222222-2222-2222-2222-222222222222"

type fakeRepository struct {
	cards      []Card
	total      int
	findAllErr error
	lastUserID string
	lastFilter CardFilter

	findByIDCard Card
	findByIDErr  error

	createdCard Card
	createErr   error

	updatedCard Card
	updateErr   error

	deleteErr error
}

func (f *fakeRepository) FindAll(ctx context.Context, userID string, filter CardFilter) ([]Card, int, error) {
	f.lastUserID = userID
	f.lastFilter = filter
	return f.cards, f.total, f.findAllErr
}

func (f *fakeRepository) FindByID(ctx context.Context, userID string, id int) (Card, error) {
	f.lastUserID = userID
	if f.findByIDErr != nil {
		return Card{}, f.findByIDErr
	}
	return f.findByIDCard, nil
}

func (f *fakeRepository) Create(ctx context.Context, userID string, c Card) (Card, error) {
	f.lastUserID = userID
	if f.createErr != nil {
		return Card{}, f.createErr
	}
	f.createdCard = c
	c.ID = 1
	return c, nil
}

func (f *fakeRepository) Update(ctx context.Context, userID string, c Card) (Card, error) {
	f.lastUserID = userID
	if f.updateErr != nil {
		return Card{}, f.updateErr
	}
	f.updatedCard = c
	return c, nil
}

func (f *fakeRepository) Delete(ctx context.Context, userID string, id int) error {
	f.lastUserID = userID
	return f.deleteErr
}

func TestService_GetAllCards_PassesUserIDAndFilterToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	filter := CardFilter{Page: 1, Limit: 25}
	_, _, err := service.GetAllCards(context.Background(), testUserID, filter)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
	assert.Equal(t, filter, repo.lastFilter)
}

func TestService_GetAllCards_ReturnsCardsAndTotalFromRepository(t *testing.T) {
	expected := []Card{
		{ID: 1, Name: "Black Lotus", SetCode: "lea"},
		{ID: 2, Name: "Lightning Bolt", SetCode: "2xm"},
	}
	repo := &fakeRepository{cards: expected, total: 2}
	service := NewService(repo)

	result, total, err := service.GetAllCards(context.Background(), testUserID, CardFilter{Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, expected, result)
	assert.Equal(t, 2, total)
}

func TestService_GetAllCards_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findAllErr: errors.New("connection lost")}
	service := NewService(repo)

	result, total, err := service.GetAllCards(context.Background(), testUserID, CardFilter{Page: 1, Limit: 25})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, total)
}

func TestService_GetCard_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.GetCard(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_GetCard_ReturnsNotFoundWhenCardBelongsToAnotherUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetCard(context.Background(), otherUserID, 1)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_CreateCard_PassesUserIDAndCardToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	storageID := 2
	input := Card{
		Name:            "Sol Ring",
		ScryfallID:      "f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f",
		SetCode:         "cmr",
		CollectorNumber: "322",
		Foil:            false,
		StorageID:       &storageID,
	}

	_, err := service.CreateCard(context.Background(), testUserID, input)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
	assert.Equal(t, input, repo.createdCard)
}

func TestService_CreateCard_ReturnsCardFromRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	result, err := service.CreateCard(context.Background(), testUserID, Card{Name: "Sol Ring", SetCode: "cmr"})

	require.NoError(t, err)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, "Sol Ring", result.Name)
}

func TestService_CreateCard_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{createErr: errors.New("insert failed")}
	service := NewService(repo)

	result, err := service.CreateCard(context.Background(), testUserID, Card{Name: "Sol Ring"})

	assert.Error(t, err)
	assert.Equal(t, Card{}, result)
}

func TestService_UpdateCard_AppliesPartialChangesOnExistingCard(t *testing.T) {
	existing := Card{ID: 1, Name: "Black Lotus", SetCode: "lea"}
	repo := &fakeRepository{findByIDCard: existing}
	service := NewService(repo)

	newName := "Renamed"
	req := updateCardRequest{Name: &newName}

	result, err := service.UpdateCard(context.Background(), testUserID, 1, req)

	require.NoError(t, err)
	assert.Equal(t, "Renamed", result.Name)
	assert.Equal(t, "lea", result.SetCode)
}

func TestService_UpdateCard_AppliesAllFieldsWhenProvided(t *testing.T) {
	existing := Card{ID: 1, Name: "Black Lotus", ScryfallID: "old-id", SetCode: "lea", CollectorNumber: "1", Foil: false}
	repo := &fakeRepository{findByIDCard: existing}
	service := NewService(repo)

	newName := "Renamed"
	newScryfallID := "11111111-1111-1111-1111-111111111111"
	newSetCode := "2ed"
	newCollectorNumber := "233"
	newFoil := true
	newStorageID := 7
	req := updateCardRequest{
		Name:            &newName,
		ScryfallID:      &newScryfallID,
		SetCode:         &newSetCode,
		CollectorNumber: &newCollectorNumber,
		Foil:            &newFoil,
		StorageID:       &newStorageID,
	}

	result, err := service.UpdateCard(context.Background(), testUserID, 1, req)

	require.NoError(t, err)
	assert.Equal(t, "Renamed", result.Name)
	assert.Equal(t, newScryfallID, result.ScryfallID)
	assert.Equal(t, "2ed", result.SetCode)
	assert.Equal(t, "233", result.CollectorNumber)
	assert.True(t, result.Foil)
	require.NotNil(t, result.StorageID)
	assert.Equal(t, 7, *result.StorageID)
}

func TestService_UpdateCard_PassesUserIDToFindAndUpdate(t *testing.T) {
	repo := &fakeRepository{findByIDCard: Card{ID: 1, Name: "Black Lotus"}}
	service := NewService(repo)

	newName := "Renamed"
	_, err := service.UpdateCard(context.Background(), testUserID, 1, updateCardRequest{Name: &newName})

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_UpdateCard_ReturnsNotFoundWhenCardDoesNotBelongToUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	newName := "Doesn't matter"
	req := updateCardRequest{Name: &newName}

	_, err := service.UpdateCard(context.Background(), otherUserID, 999, req)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_UpdateCard_PropagatesRepositoryUpdateError(t *testing.T) {
	existing := Card{ID: 1, Name: "Black Lotus", SetCode: "lea"}
	repo := &fakeRepository{findByIDCard: existing, updateErr: errors.New("update failed")}
	service := NewService(repo)

	newName := "New Name"
	req := updateCardRequest{Name: &newName}

	_, err := service.UpdateCard(context.Background(), testUserID, 1, req)

	assert.Error(t, err)
}

func TestService_DeleteCard_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	err := service.DeleteCard(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_DeleteCard_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{deleteErr: ErrNotFound}
	service := NewService(repo)

	err := service.DeleteCard(context.Background(), testUserID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_DeleteCard_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{deleteErr: errors.New("delete failed")}
	service := NewService(repo)

	err := service.DeleteCard(context.Background(), testUserID, 1)

	assert.Error(t, err)
}
