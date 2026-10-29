package deckinsights

import "errors"

var ErrUnknownFormat = errors.New("deck format is not a format scryfall recognizes")

type LegalityIssue struct {
	CardID   int    `json:"card_id,omitempty"`
	CardName string `json:"card_name"`
	Board    string `json:"board,omitempty"`
	Reason   string `json:"reason"`
}

type LegalityReport struct {
	Format string          `json:"format"`
	Legal  bool            `json:"legal"`
	Issues []LegalityIssue `json:"issues,omitempty"`
}

type ManaCurveCard struct {
	Name       string `json:"name"`
	ScryfallID string `json:"scryfall_id"`
	Quantity   int    `json:"quantity"`
	Type       string `json:"type"`
}

type ManaCurveBucket struct {
	ManaValue     int             `json:"mana_value"`
	Count         int             `json:"count"`
	Permanents    int             `json:"permanents"`
	NonPermanents int             `json:"non_permanents"`
	Cards         []ManaCurveCard `json:"cards"`
}

type BackFaceCard struct {
	Name       string `json:"name"`
	ScryfallID string `json:"scryfall_id"`
	Quantity   int    `json:"quantity"`
}

type DeckStats struct {
	CardCount        int               `json:"card_count"`
	LandCount        int               `json:"land_count"`
	NonlandCount     int               `json:"nonland_count"`
	AverageManaValue float64           `json:"average_mana_value"`
	TotalManaValue   float64           `json:"total_mana_value"`
	ManaCurve        []ManaCurveBucket `json:"mana_curve"`
	ColorBreakdown   map[string]int    `json:"color_breakdown"`
	TypeBreakdown    map[string]int    `json:"type_breakdown"`
	BackFaceLands    []BackFaceCard    `json:"back_face_lands"`
	Value            Value             `json:"value"`
	OwnedValue       Value             `json:"owned_value"`
	PendingValue     Value             `json:"pending_value"`
	BoardValues      BoardValues       `json:"board_values"`
	// GameChangerCount is the number of distinct cards of the main board
	// whose stored printing is a game changer.
	GameChangerCount int `json:"game_changer_count"`
}

type BoardValues struct {
	Sideboard   Value `json:"sideboard"`
	Considering Value `json:"considering"`
}

type Value struct {
	EUR float64 `json:"eur"`
	USD float64 `json:"usd"`
}

// Printing is what deck statistics and legality know about a card, as stored
// in tamiyo.printings by the background refresher.
type Printing struct {
	ScryfallID        string
	Name              string
	TypeLine          string
	OracleText        string
	Keywords          []string
	ManaValue         float64
	Colors            []string
	ColorIdentity     []string
	LegalFormats      []string // legal or restricted
	BannedFormats     []string
	RestrictedFormats []string
}

// DeckValues holds a deck's values before rounding, priced by tamiyo.card_price.
type DeckValues struct {
	Owned       Value
	Pending     Value
	Sideboard   Value
	Considering Value
}

func (v Value) Plus(other Value) Value {
	return Value{EUR: v.EUR + other.EUR, USD: v.USD + other.USD}
}

// DeckHistory is the net change in copies of a deck, day by day, newest day
// first.
type DeckHistory struct {
	Days []HistoryDay `json:"days"`
}

type HistoryDay struct {
	Date   string         `json:"date"`
	Boards []HistoryBoard `json:"boards"`
}

type HistoryBoard struct {
	Board   string          `json:"board"`
	Changes []HistoryChange `json:"changes"`
}

// HistoryChange is a card name's net balance: positive for an addition,
// negative for a removal, never zero.
type HistoryChange struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	// SetCode, CollectorNumber and Foil name a copy behind the line (an
	// added one for an addition, a removed one for a removal); nil on lines
	// recorded before they were remembered.
	SetCode         *string `json:"set_code,omitempty"`
	CollectorNumber *string `json:"collector_number,omitempty"`
	Foil            *bool   `json:"foil,omitempty"`
}

// HistoryRow is one stored balance of deck_history.
type HistoryRow struct {
	Date  string
	Board string
	Name  string
	Count int

	SetCode         *string `db:"set_code"`
	CollectorNumber *string `db:"collector_number"`
	Foil            *bool   `db:"foil"`
}
