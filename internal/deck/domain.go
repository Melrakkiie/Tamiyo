package deck

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var ErrCommanderNotFound = errors.New("referenced commander does not exist")
var ErrPartnerWithoutCommander = errors.New("a deck needs a commander before a second commander")
var ErrSameCommanders = errors.New("the commander and the second commander must be different cards")
var ErrCardNotFound = errors.New("referenced card does not exist")
var ErrNotFound = errors.New("deck not found")
var ErrBackgroundFaceWithoutPrinting = errors.New("background_face needs background_scryfall_id")

// Faces of a printing the deck art can show.
const (
	FaceFront = "front"
	FaceBack  = "back"
)

type Deck struct {
	ID                   string
	Name                 string
	Format               string
	CommanderID          *int
	CommanderPendingID   *int
	PartnerID            *int
	PartnerPendingID     *int
	BackgroundScryfallID *string
	BackgroundFace       string
	CommanderScryfallID  *string
	PartnerScryfallID    *string
	ColorIdentity        *string
	Visibility           string
	Bracket              *int
	CardCount            int
	PendingCount         int
	LikesCount           int
	FolderID             *int
	Favorite             bool
	Added                time.Time
	Updated              time.Time
}

type CommanderSlot int

const (
	SlotNone CommanderSlot = iota
	SlotCommander
	SlotPartner
)

func sameID(a *int, id int) bool {
	return a != nil && *a == id
}

func (d Deck) CardSlot(cardID int) (CommanderSlot, bool) {
	switch {
	case sameID(d.CommanderID, cardID):
		return SlotCommander, true
	case sameID(d.PartnerID, cardID):
		return SlotPartner, true
	}
	return SlotNone, false
}

func (d Deck) PendingSlot(pendingID int) (CommanderSlot, bool) {
	switch {
	case sameID(d.CommanderPendingID, pendingID):
		return SlotCommander, true
	case sameID(d.PartnerPendingID, pendingID):
		return SlotPartner, true
	}
	return SlotNone, false
}

func (d Deck) IsCommanderCard(cardID int) bool {
	_, ok := d.CardSlot(cardID)
	return ok
}

func (d Deck) IsCommanderPending(pendingID int) bool {
	_, ok := d.PendingSlot(pendingID)
	return ok
}

func (d Deck) HasCommander() bool {
	return d.CommanderID != nil || d.CommanderPendingID != nil
}

func (d Deck) HasPartner() bool {
	return d.PartnerID != nil || d.PartnerPendingID != nil
}

func (d *Deck) setSlot(slot CommanderSlot, cardID, pendingID *int) {
	if slot == SlotPartner {
		d.PartnerID, d.PartnerPendingID = cardID, pendingID
		return
	}
	d.CommanderID, d.CommanderPendingID = cardID, pendingID
}

func (d *Deck) clearSlot(slot CommanderSlot) {
	d.setSlot(slot, nil, nil)
	if !d.HasCommander() {
		d.setSlot(SlotCommander, d.PartnerID, d.PartnerPendingID)
		d.setSlot(SlotPartner, nil, nil)
	}
}

func (d Deck) checkCommanders() error {
	if d.HasPartner() && !d.HasCommander() {
		return ErrPartnerWithoutCommander
	}
	if d.PartnerID != nil && sameID(d.CommanderID, *d.PartnerID) || d.PartnerPendingID != nil && sameID(d.CommanderPendingID, *d.PartnerPendingID) {
		return ErrSameCommanders
	}
	return nil
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
	Price           *float64
	GameChanger     bool
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

// MainBoardSQL is InMainBoard as an SQL condition on the board column of the
// table alias, or on the bare board column when alias is empty.
func MainBoardSQL(alias string) string {
	column := "board"
	if alias != "" {
		column = alias + ".board"
	}
	return column + " NOT IN ('" + BoardSideboard + "', '" + BoardConsidering + "')"
}

// CardCountSQL is a deck's card count for the alias of its deck table: every
// copy in its main board, owned or pending.
func CardCountSQL(deck string) string {
	return "((SELECT count(*) FROM tamiyo.card_deck own WHERE own.deck_id = " + deck + ".id AND " + MainBoardSQL("own") + ")" +
		" + (SELECT COALESCE(SUM(pen.quantity), 0) FROM tamiyo.deck_pending_cards pen WHERE pen.deck_id = " + deck + ".id AND " + MainBoardSQL("pen") + "))"
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
	Composer
	PublicRepository
	CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error)
	FindOwnedCopies(ctx context.Context, userID string, targetDeckID string, scryfallIDs []string, nameKeys []string) ([]OwnedCopy, error)
	TagRepository
	ViewRepository
	LikeRepository
	FolderRepository
}
