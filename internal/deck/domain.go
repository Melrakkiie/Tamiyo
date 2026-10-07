package deck

import (
	"context"
	"errors"
	"time"
)

var ErrCommanderNotFound = errors.New("referenced commander does not exist")
var ErrCardNotFound = errors.New("referenced card does not exist")
var ErrNotFound = errors.New("deck not found")

type Deck struct {
	ID                   int
	Name                 string
	Format               string
	CommanderID          *int
	CommanderPendingID   *int
	BackgroundScryfallID *string
	CommanderScryfallID  *string
	CardCount            int
	PendingCount         int
	Added                time.Time
	Updated              time.Time
}

type DeckCard struct {
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
	ColorIdentity   *string
	Added           time.Time
	Updated         time.Time
}

type Filter struct {
	Format string

	SortField string
	SortDesc  bool

	Page  int
	Limit int
}

type Repository interface {
	FindAll(ctx context.Context, userID string, filter Filter) ([]Deck, int, error)
	FindByID(ctx context.Context, userID string, id int) (Deck, error)
	Create(ctx context.Context, userID string, d Deck) (Deck, error)
	Update(ctx context.Context, userID string, d Deck) (Deck, error)
	Delete(ctx context.Context, userID string, id int) error

	FindCardsByDeckID(ctx context.Context, userID string, id int, sortField string, sortDesc bool) ([]DeckCard, error)
	LinkCardToDeck(ctx context.Context, userID string, deckID int, cardID int) error
	UnlinkCardFromDeck(ctx context.Context, userID string, deckID int, cardID int) error

	PendingRepository
}
