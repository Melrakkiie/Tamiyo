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

func (f *fakeRepository) Delete(ctx context.Context, userID string, id int) error {
	f.lastUserID = userID
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

	err := service.DeleteStorage(context.Background(), testUserID, 1)

	require.NoError(t, err)
	assert.Equal(t, testUserID, repo.lastUserID)
}

func TestService_DeleteStorage_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{deleteErr: ErrNotFound}
	service := NewService(repo)

	err := service.DeleteStorage(context.Background(), testUserID, 999)

	assert.ErrorIs(t, err, ErrNotFound)
}
