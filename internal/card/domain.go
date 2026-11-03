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
	Proxy           bool
	StorageID       *int
	ManaValue       float64
	Colors          *string
	CardType        *string
	ColorIdentity   *string
	Subtype         *string
	Added           time.Time
	Updated         time.Time
	CopyIDs         []int
}

// ImportPlan is everything a collection import writes, in one transaction.
// Later tickets add the decks to create and the references from copies to them.
type ImportPlan struct {
	Storages []NewStorage
	Copies   []ImportCopy
}

// NewStorage is a storage the import creates; copies point at it by its index in ImportPlan.Storages.
type NewStorage struct {
	Name string
	Type string
}

// ImportCopy is one physical copy to create. Card.StorageID is an existing storage;
// NewStorage, when set, is the index of a storage of the plan and wins over Card.StorageID.
type ImportCopy struct {
	Card       Card
	NewStorage *int
}

// ImportResult holds what the plan created; Cards follow the order of ImportPlan.Copies
// and StorageIDs the order of ImportPlan.Storages.
type ImportResult struct {
	Cards      []Card
	StorageIDs []int
}

type CardFilter struct {
	StorageID     *int
	ScryfallID    string
	Name          string
	Query         *SearchNode
	ColorIdentity *string
	Stack         bool

	OpenToDeck *string

	GroupBy   string
	SortField string
	SortDesc  bool

	Page  int
	Limit int
}

type Repository interface {
	FindAll(ctx context.Context, userID string, filter CardFilter) ([]Card, int, error)
	FindByID(ctx context.Context, userID string, id int) (Card, error)
	Create(ctx context.Context, userID string, c Card) (Card, error)
	ImportCollection(ctx context.Context, userID string, plan ImportPlan) (ImportResult, error)
	Update(ctx context.Context, userID string, c Card) (Card, error)
	Delete(ctx context.Context, userID string, id int) error
	DeleteAll(ctx context.Context, userID string) (int, error)
	FindMissingDetails(ctx context.Context, userID string, afterID int, limit int) ([]Card, error)
	CountMissingDetails(ctx context.Context, userID string, afterID int) (int, error)
	SetDetails(ctx context.Context, userID string, id int, details Details) error
	MoveToStorage(ctx context.Context, userID string, ids []int, storageID int) (int, error)
	MoveStorageCards(ctx context.Context, userID string, fromStorageID, toStorageID int) (int, error)
	Value(ctx context.Context, userID string) (Value, error)
}

type Value struct {
	EUR float64
	USD float64
}

type Details struct {
	Colors        string
	CardType      string
	ColorIdentity string
	ManaValue     float64
}

var comparisonOperators = map[string]string{
	"eq":  "=",
	"lt":  "<",
	"lte": "<=",
	"gt":  ">",
	"gte": ">=",
}

func IsLegalFormat(format string) bool {
	return legalFormats[format]
}

var legalFormats = map[string]bool{
	"standard": true, "future": true, "historic": true, "timeless": true, "gladiator": true,
	"pioneer": true, "explorer": true, "modern": true, "legacy": true, "pauper": true,
	"vintage": true, "penny": true, "commander": true, "oathbreaker": true, "standardbrawl": true,
	"brawl": true, "alchemy": true, "paupercommander": true, "duel": true, "oldschool": true,
	"premodern": true, "predh": true,
}
