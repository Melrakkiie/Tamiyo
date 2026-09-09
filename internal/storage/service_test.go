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
	storageID  *int

	createdStorage Storage
	createErr      error
}

func (f *fakeRepository) FindAll(ctx context.Context) ([]Storage, error) {
	return f.storages, f.findAllErr
}

func (f *fakeRepository) Create(ctx context.Context, storage Storage) (Storage, error) {
	if f.createErr != nil {
		return Storage{}, f.createErr
	}
	f.createdStorage = storage
	storage.ID = 1
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
