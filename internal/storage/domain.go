package storage

import (
	"context"
	"time"
)

type Storage struct {
	ID    int
	Name  string
	Type  string
	Added time.Time
}

type Repository interface {
	FindAll(ctx context.Context) ([]Storage, error)
}
