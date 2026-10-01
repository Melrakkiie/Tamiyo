package deckinsights

import "errors"

var ErrDeckNotFound = errors.New("deck does not exist")
var ErrUnknownFormat = errors.New("deck format is not a format scryfall recognizes")
var ErrScryfallUnavailable = errors.New("could not resolve cards against scryfall")

type LegalityIssue struct {
	CardID   int    `json:"card_id,omitempty"`
	CardName string `json:"card_name"`
	Reason   string `json:"reason"`
}

type LegalityReport struct {
	Format string          `json:"format"`
	Legal  bool            `json:"legal"`
	Issues []LegalityIssue `json:"issues,omitempty"`
}

type ManaCurveBucket struct {
	ManaValue int `json:"mana_value"`
	Count     int `json:"count"`
}

type DeckStats struct {
	CardCount        int               `json:"card_count"`
	LandCount        int               `json:"land_count"`
	NonlandCount     int               `json:"nonland_count"`
	AverageManaValue float64           `json:"average_mana_value"`
	ManaCurve        []ManaCurveBucket `json:"mana_curve"`
	ColorBreakdown   map[string]int    `json:"color_breakdown"`
	TypeBreakdown    map[string]int    `json:"type_breakdown"`
}
