package storage

import (
	"context"
	"time"
)

type Storage struct {
	ID        int
	Name      string
	Type      string
	CardCount int
	Added     time.Time
}

type Repository interface {
	FindAll(ctx context.Context) ([]Storage, error)
	Create(ctx context.Context, storage Storage) (Storage, error)
}
