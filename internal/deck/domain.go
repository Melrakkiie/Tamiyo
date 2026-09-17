package deck

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("deck not found")

type Deck struct {
	ID          int
	Name        string
	Format      string
	CommanderID *int
	CardCount   int
	Added       time.Time
	Updated     time.Time
}

type Repository interface {
	FindAll(ctx context.Context) ([]Deck, error)
	FindByID(ctx context.Context, id int) (Deck, error)
}
