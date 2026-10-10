package deck

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var ErrCommanderNotFound = errors.New("referenced commander does not exist")
var ErrCardNotFound = errors.New("referenced card does not exist")
var ErrNotFound = errors.New("deck not found")

type Deck struct {
	ID                   string
	Name                 string
	Format               string
	CommanderID          *int
	CommanderPendingID   *int
	BackgroundScryfallID *string
	CommanderScryfallID  *string
	Visibility           string
	CardCount            int
	PendingCount         int
	LikesCount           int
	Added                time.Time
	Updated              time.Time
}

const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
)

type DeckCard struct {
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
	Board           string
	Added           time.Time
	Updated         time.Time
}

const (
	BoardMain        = "main"
	BoardSideboard   = "sideboard"
	BoardConsidering = "considering"
)

func InMainBoard(board string) bool {
	return board != BoardSideboard && board != BoardConsidering
}

var ErrCommanderBoard = errors.New("the commander stays in the main deck")

type Filter struct {
	Format     string
	Visibility string

	SortField string
	SortDesc  bool

	Page  int
	Limit int
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ParseID(raw string) (string, bool) {
	id := strings.ToLower(raw)
	return id, idPattern.MatchString(id)
}

type Repository interface {
	FindAll(ctx context.Context, userID string, filter Filter) ([]Deck, int, error)
	FindByID(ctx context.Context, userID string, id string) (Deck, error)
	Create(ctx context.Context, userID string, d Deck) (Deck, error)
	Update(ctx context.Context, userID string, d Deck) (Deck, error)
	Delete(ctx context.Context, userID string, id string) error
	FindShared(ctx context.Context, id string) (string, Deck, error)

	FindCardsByDeckID(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]DeckCard, error)
	LinkCardToDeck(ctx context.Context, userID string, deckID string, cardID int, board string) error
	UnlinkCardFromDeck(ctx context.Context, userID string, deckID string, cardID int) error

	PendingRepository
	PublicRepository
	CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error)
	TagRepository
	ViewRepository
	LikeRepository
}
