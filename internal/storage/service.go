package storage

import (
	"context"
	"time"
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

func (s *Service) CreateStorage(ctx context.Context, storage Storage) (Storage, error) {
	storage.Added = time.Now()
	return s.repo.Create(ctx, storage)
}
