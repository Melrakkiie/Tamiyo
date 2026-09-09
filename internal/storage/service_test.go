package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	storages   []Storage
	findAllErr error

	findByIDStorage Storage
	findByIDErr     error

	createdStorage Storage
	createErr      error

	updatedStorage Storage
	updateErr      error
}

func (f *fakeRepository) FindAll(ctx context.Context) ([]Storage, error) {
	return f.storages, f.findAllErr
}

func (f *fakeRepository) FindByID(ctx context.Context, id int) (Storage, error) {
	if f.findByIDErr != nil {
		return Storage{}, f.findByIDErr
	}
	return f.findByIDStorage, nil
}

func (f *fakeRepository) Create(ctx context.Context, storage Storage) (Storage, error) {
	if f.createErr != nil {
		return Storage{}, f.createErr
	}
	f.createdStorage = storage
	storage.ID = 1
	return storage, nil
}

func (f *fakeRepository) Update(ctx context.Context, storage Storage) (Storage, error) {
	if f.updateErr != nil {
		return Storage{}, f.updateErr
	}
	f.updatedStorage = storage
	return storage, nil
}

func TestService_GetAllStorages_ReturnsStoragesFromRepository(t *testing.T) {
	expected := []Storage{
		{ID: 1, Name: "Vintage Collection", Type: "binder"},
		{ID: 2, Name: "Tarkir", Type: "box"},
	}
	repo := &fakeRepository{storages: expected}
	service := NewService(repo)

	result, err := service.GetAllStorages(context.Background())

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetAllStorages_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{findAllErr: errors.New("connection lost")}
	service := NewService(repo)

	result, err := service.GetAllStorages(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestService_GetStorage_ReturnsStorageFromRepository(t *testing.T) {
	expected := Storage{ID: 1, Name: "Vintage Collection", Type: "binder"}
	repo := &fakeRepository{findByIDStorage: expected}
	service := NewService(repo)

	result, err := service.GetStorage(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestService_GetStorage_PropagatesNotFoundError(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetStorage(context.Background(), 999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_CreateStorage_PassesStorageUnchangedToRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	input := Storage{
		Name: "Vintage Collection",
		Type: "binder",
	}

	_, err := service.CreateStorage(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, input, repo.createdStorage)
}

func TestService_CreateStorage_ReturnsStorageFromRepository(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	result, err := service.CreateStorage(context.Background(), Storage{Name: "Vintage Collection", Type: "binder"})

	require.NoError(t, err)
	assert.Equal(t, 1, result.ID)
	assert.Equal(t, "Vintage Collection", result.Name)
}

func TestService_CreateStorage_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeRepository{createErr: errors.New("insert failed")}
	service := NewService(repo)

	result, err := service.CreateStorage(context.Background(), Storage{Name: "Vintage Collection"})

	assert.Error(t, err)
	assert.Equal(t, Storage{}, result)
}

func TestService_UpdateStorage_AppliesPartialChangesOnExistingStorage(t *testing.T) {
	existing := Storage{ID: 1, Name: "Vintage Collection", Type: "binder"}
	repo := &fakeRepository{findByIDStorage: existing}
	service := NewService(repo)

	newName := "Vintage Collection Renamed"
	req := updateStorageRequest{Name: &newName}

	result, err := service.UpdateStorage(context.Background(), 1, req)

	require.NoError(t, err)
	assert.Equal(t, "Vintage Collection Renamed", result.Name)
	assert.Equal(t, "binder", result.Type) // inchangé, non fourni dans la requête
}

func TestService_UpdateStorage_ReturnsNotFoundWhenStorageDoesNotExist(t *testing.T) {
	repo := &fakeRepository{findByIDErr: ErrNotFound}
	service := NewService(repo)

	newName := "Doesn't matter"
	req := updateStorageRequest{Name: &newName}

	_, err := service.UpdateStorage(context.Background(), 999, req)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_UpdateStorage_PropagatesRepositoryUpdateError(t *testing.T) {
	existing := Storage{ID: 1, Name: "Vintage Collection", Type: "binder"}
	repo := &fakeRepository{findByIDStorage: existing, updateErr: errors.New("update failed")}
	service := NewService(repo)

	newName := "New Name"
	req := updateStorageRequest{Name: &newName}

	_, err := service.UpdateStorage(context.Background(), 1, req)

	assert.Error(t, err)
}
