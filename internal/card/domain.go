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
	ManaValue       float64
	Colors          *string
	CardType        *string
	Added           time.Time
	Updated         time.Time
}

type CardFilter struct {
	StorageID *int
	Name      string

	SortField string
	SortDesc  bool

	Page  int
	Limit int
}

type Repository interface {
	FindAll(ctx context.Context, userID string, filter CardFilter) ([]Card, int, error)
	FindByID(ctx context.Context, userID string, id int) (Card, error)
	Create(ctx context.Context, userID string, c Card) (Card, error)
	Update(ctx context.Context, userID string, c Card) (Card, error)
	Delete(ctx context.Context, userID string, id int) error
	DeleteAll(ctx context.Context, userID string) (int, error)
	FindMissingDetails(ctx context.Context, userID string, afterID int, limit int) ([]Card, error)
	CountMissingDetails(ctx context.Context, userID string, afterID int) (int, error)
	SetDetails(ctx context.Context, userID string, id int, details Details) error
}

type Details struct {
	Colors    string
	CardType  string
	ManaValue float64
}
