package storage

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("storage not found")

type Storage struct {
	ID        int
	Name      string
	Type      string
	CardCount int
	Added     time.Time
	Updated   time.Time
}

type Filter struct {
	Type string

	Page  int
	Limit int
}

type Repository interface {
	FindAll(ctx context.Context, userID string, filter Filter) ([]Storage, int, error)
	FindByID(ctx context.Context, userID string, id int) (Storage, error)
	Create(ctx context.Context, userID string, storage Storage) (Storage, error)
	Update(ctx context.Context, userID string, storage Storage) (Storage, error)
	Delete(ctx context.Context, userID string, id int) error
}
