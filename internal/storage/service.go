package storage

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetAllStorages(ctx context.Context, userID string) ([]Storage, error) {
	return s.repo.FindAll(ctx, userID)
}

func (s *Service) GetStorage(ctx context.Context, userID string, id int) (Storage, error) {
	return s.repo.FindByID(ctx, userID, id)
}

func (s *Service) CreateStorage(ctx context.Context, userID string, storage Storage) (Storage, error) {
	return s.repo.Create(ctx, userID, storage)
}

func (s *Service) UpdateStorage(ctx context.Context, userID string, id int, req updateStorageRequest) (Storage, error) {
	existing, err := s.repo.FindByID(ctx, userID, id)
	if err != nil {
		return Storage{}, err
	}

	updated := req.applyTo(existing)

	return s.repo.Update(ctx, userID, updated)
}

func (s *Service) DeleteStorage(ctx context.Context, userID string, id int) error {
	return s.repo.Delete(ctx, userID, id)
}
