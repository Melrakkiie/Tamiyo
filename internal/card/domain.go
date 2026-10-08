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
	Added           time.Time
	Updated         time.Time
	CopyIDs         []int
}

type CardFilter struct {
	StorageID     *int
	Name          string
	ColorIdentity *string
	Stack         bool

	Colors      *string
	ColorMode   string
	ManaValue   *float64
	ManaValueOp string
	Type        string
	Subtype     string
	LegalIn     string
	ColorCount  *int
	Foil        *bool
	StorageType string

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
	Update(ctx context.Context, userID string, c Card) (Card, error)
	Delete(ctx context.Context, userID string, id int) error
	DeleteAll(ctx context.Context, userID string) (int, error)
	FindMissingDetails(ctx context.Context, userID string, afterID int, limit int) ([]Card, error)
	CountMissingDetails(ctx context.Context, userID string, afterID int) (int, error)
	SetDetails(ctx context.Context, userID string, id int, details Details) error
}

type Details struct {
	Colors        string
	CardType      string
	ColorIdentity string
	ManaValue     float64
}

const (
	ColorModeExact   = "exact"
	ColorModeInclude = "include"
	ColorModeWithin  = "within"
)

var manaValueOperators = map[string]string{
	"eq":  "=",
	"lt":  "<",
	"lte": "<=",
	"gt":  ">",
	"gte": ">=",
}

var legalFormats = map[string]bool{
	"standard": true, "future": true, "historic": true, "timeless": true, "gladiator": true,
	"pioneer": true, "explorer": true, "modern": true, "legacy": true, "pauper": true,
	"vintage": true, "penny": true, "commander": true, "oathbreaker": true, "standardbrawl": true,
	"brawl": true, "alchemy": true, "paupercommander": true, "duel": true, "oldschool": true,
	"premodern": true, "predh": true,
}
