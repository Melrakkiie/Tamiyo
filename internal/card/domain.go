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

// Repository définit le contrat de persistence attendu par le domaine.
// L'implémentation concrète (Postgres, mémoire, etc.) vit dans la couche infrastructure.
type Repository interface {
	FindAll(ctx context.Context) ([]Card, error)
}
