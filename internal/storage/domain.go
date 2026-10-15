package storage

import (
	"context"
	"errors"
	"slices"
	"time"
)

var (
	ErrNotFound     = errors.New("storage not found")
	ErrDeckNotFound = errors.New("allowed_deck_ids must only contain your own decks")
)

type Storage struct {
	ID        int
	Name      string
	Type      string
	CardCount int
	Value     Value
	Added     time.Time
	Updated   time.Time

	Locked         bool
	AllowedDeckIDs []string
}

type Value struct {
	EUR float64
	USD float64
}

func (s Storage) OpenTo(deckID string) bool {
	return !s.Locked || slices.Contains(s.AllowedDeckIDs, deckID)
}

type Filter struct {
	Type string

	SortField string
	SortDesc  bool

	Page  int
	Limit int
}

type Repository interface {
	FindAll(ctx context.Context, userID string, filter Filter) ([]Storage, int, error)
	FindByID(ctx context.Context, userID string, id int) (Storage, error)
	Create(ctx context.Context, userID string, storage Storage) (Storage, error)
	Update(ctx context.Context, userID string, storage Storage) (Storage, error)
	Delete(ctx context.Context, userID string, id int, withCards bool) error
}
