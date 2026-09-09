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

type Repository interface {
	FindAll(ctx context.Context) ([]Storage, error)
	FindByID(ctx context.Context, id int) (Storage, error)
	Create(ctx context.Context, storage Storage) (Storage, error)
	Update(ctx context.Context, storage Storage) (Storage, error)
}
