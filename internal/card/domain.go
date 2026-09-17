package card

import (
	"context"
	"errors"
	"time"
)

var ErrStorageNotFound = errors.New("referenced storage does not exist")

type Card struct {
	ID              int
	Name            string
	ScryfallID      string
	SetCode         string
	CollectorNumber int
	Foil            bool
	StorageID       *int
	Added           time.Time
	Updated         time.Time
}

type Repository interface {
	FindAll(ctx context.Context, storageID *int) ([]Card, error)
	Create(ctx context.Context, c Card) (Card, error)
}
