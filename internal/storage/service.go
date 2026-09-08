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
