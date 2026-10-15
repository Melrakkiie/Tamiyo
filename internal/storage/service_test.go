package storage

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
	storages     []Storage
	findAllTotal int
	findAllErr   error
	lastUserID   string
	lastFilter   Filter

	lastWithCards bool

	findByIDStorage Storage
	findByIDErr     error

	createdStorage Storage
	createErr      error

	updatedStorage Storage
	updateErr      error

	deleteErr error
}

func (f *fakeRepository) FindAll(ctx context.Context, userID string, filter Filter) ([]Storage, int, error) {
	f.lastUserID = userID
	f.lastFilter = filter
	return f.storages, f.findAllTotal, f.findAllErr
}

func (f *fakeRepository) FindByID(ctx context.Context, userID string, id int) (Storage, error) {
	f.lastUserID = userID
	if f.findByIDErr != nil {
		return Storage{}, f.findByIDErr
	}
	return f.findByIDStorage, nil
}

func (f *fakeRepository) Create(ctx context.Context, userID string, storage Storage) (Storage, error) {
	f.lastUserID = userID
	if f.createErr != nil {
		return Storage{}, f.createErr
	}
	f.createdStorage = storage
	storage.ID = 1
	return storage, nil
}

func (f *fakeRepository) Update(ctx context.Context, userID string, storage Storage) (Storage, error) {
	f.lastUserID = userID
	if f.updateErr != nil {
		return Storage{}, f.updateErr
	}
	f.updatedStorage = storage
	return storage, nil
}

func (f *fakeRepository) Delete(ctx context.Context, userID string, id int, withCards bool) error {
	f.lastUserID = userID
	f.lastWithCards = withCards
	return f.deleteErr
}

func TestService_GetAllStorages_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, _, err := service.GetAllStorages(context.Background(), testUserID, Filter{})

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_GetAllStorages_PassesFilterToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, _, err := service.GetAllStorages(context.Background(), testUserID, Filter{Type: "binder", Page: 1, Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, Filter{Type: "binder", Page: 1, Limit: 25}, repo.lastFilter)
}

func TestService_GetAllStorages_ReturnsStoragesAndTotalFromRepository(t *testing.T) {
	expected := []Storage{{ID: 1, Name: "Vintage Collection", Type: "binder"}}
	repo := &fakeRepository{storages: expected, findAllTotal: 1}
	service := NewService(repo)

	result, total, err := service.GetAllStorages(context.Background(), testUserID, Filter{})

	require.NoError(t, err)
	assert.Equal(t, expected, result)
	assert.Equal(t, 1, total)
}

func TestService_GetAllStorages_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findAllErr: errors.New("connection lost")}
	service := NewService(repo)

	result, total, err := service.GetAllStorages(context.Background(), testUserID, Filter{})

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Zero(t, total)
}

func TestService_GetStorage_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.GetStorage(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_GetStorage_ReturnsNotFoundWhenStorageBelongsToAnotherUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetStorage(context.Background(), otherUserID, 1)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_CreateStorage_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, err := service.CreateStorage(context.Background(), testUserID, Storage{Name: "Trade Binder", Type: "binder"})

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_UpdateStorage_PassesUserIDToFindAndUpdate(t *testing.T) {
	repo := &fakeRepository{findByIDStorage: Storage{ID: 1, Name: "Old", Type: "binder"}}
	service := NewService(repo)

	newName := "New Name"
	_, err := service.UpdateStorage(context.Background(), testUserID, 1, updateStorageRequest{Name: &newName})

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_UpdateStorage_AppliesLockAndAllowedDecks(t *testing.T) {
	repo := &fakeRepository{findByIDStorage: Storage{ID: 1, Name: "Trades", Type: "box"}}
	service := NewService(repo)

	locked := true
	allowed := []string{"00000000-0000-0000-0000-000000000001"}
	_, err := service.UpdateStorage(context.Background(), testUserID, 1, updateStorageRequest{Locked: &locked, AllowedDeckIDs: &allowed})

	require.NoError(t, err)
	assert.True(t, repo.updatedStorage.Locked)
	assert.Equal(t, allowed, repo.updatedStorage.AllowedDeckIDs)
	assert.Equal(t, "Trades", repo.updatedStorage.Name)
}

func TestService_UpdateStorage_KeepsAllowedDecksWhenOnlyUnlocking(t *testing.T) {
	allowed := []string{"00000000-0000-0000-0000-000000000001"}
	repo := &fakeRepository{findByIDStorage: Storage{ID: 1, Locked: true, AllowedDeckIDs: allowed}}
	service := NewService(repo)

	unlocked := false
	_, err := service.UpdateStorage(context.Background(), testUserID, 1, updateStorageRequest{Locked: &unlocked})

	require.NoError(t, err)
	assert.False(t, repo.updatedStorage.Locked)
	assert.Equal(t, allowed, repo.updatedStorage.AllowedDeckIDs)
}

func TestStorage_OpenTo(t *testing.T) {
	deckID := "00000000-0000-0000-0000-000000000001"
	assert.True(t, Storage{}.OpenTo(deckID))
	assert.True(t, Storage{AllowedDeckIDs: []string{"other"}}.OpenTo(deckID))
	assert.False(t, Storage{Locked: true}.OpenTo(deckID))
	assert.True(t, Storage{Locked: true, AllowedDeckIDs: []string{deckID}}.OpenTo(deckID))
}

func TestService_UpdateStorage_ReturnsNotFoundWhenStorageDoesNotBelongToUser(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	newName := "New Name"
	_, err := service.UpdateStorage(context.Background(), otherUserID, 1, updateStorageRequest{Name: &newName})

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_DeleteStorage_PassesUserIDToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	err := service.DeleteStorage(context.Background(), testUserID, 1, true)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
	assert.True(t, repo.lastWithCards)
}

func TestService_DeleteStorage_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{deleteErr: ErrNotFound}
	service := NewService(repo)

	err := service.DeleteStorage(context.Background(), testUserID, 999, false)

	assert.ErrorIs(t, err, ErrNotFound)
}
