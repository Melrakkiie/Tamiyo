package storage

import (
	"context"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllStorages(ctx context.Context) ([]Storage, error) {
	return s.repo.FindAll(ctx)
}

func (s *Service) GetStorage(ctx context.Context, id int) (Storage, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) CreateStorage(ctx context.Context, storage Storage) (Storage, error) {
	return s.repo.Create(ctx, storage)
}

func (s *Service) UpdateStorage(ctx context.Context, id int, req updateStorageRequest) (Storage, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Storage{}, err
	}

	updated := req.applyTo(existing)

	return s.repo.Update(ctx, updated)
}
