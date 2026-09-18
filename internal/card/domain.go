package card

import (
	"context"
	"errors"
	"time"
)

var ErrStorageNotFound = errors.New("referenced storage does not exist")
var ErrNotFound = errors.New("card not found")

type Card struct {
	ID              int
	Name            string
	ScryfallID      string
	SetCode         string
	CollectorNumber string
	Foil            bool
	StorageID       *int
	Added           time.Time
	Updated         time.Time
}

type CardFilter struct {
	StorageID *int
	Name      string
	Page      int
	Limit     int
}

type Repository interface {
	FindAll(ctx context.Context, filter CardFilter) ([]Card, int, error)
	FindByID(ctx context.Context, id int) (Card, error)
	Create(ctx context.Context, c Card) (Card, error)
	Update(ctx context.Context, c Card) (Card, error)
	Delete(ctx context.Context, id int) error
}
