package card

import (
	"context"
	"time"
)

type Card struct {
	ID              int
	Name            string
	ScryfallID      string
	SetCode         string
	CollectorNumber int
	Foil            bool
	BinderName      string
	BinderType      string
	Added           time.Time
}

type Repository interface {
	FindAll(ctx context.Context) ([]Card, error)
	Create(ctx context.Context, c Card) (Card, error)
}
