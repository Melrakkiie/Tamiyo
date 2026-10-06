package bulk

import (
	"context"
	"errors"
	"strings"
)

var ErrInvalidFile = errors.New("could not parse the uploaded file")

var ErrTargetStorageNotFound = errors.New("target storage does not exist")

var ErrDeckNotFound = errors.New("deck does not exist")

var ErrScryfallUnavailable = errors.New("could not resolve cards against scryfall")

type Summary struct {
	CardsCreated    int      `json:"cards_created"`
	CardsSkipped    int      `json:"cards_skipped"`
	StoragesCreated int      `json:"storages_created"`
	DecksCreated    int      `json:"decks_created"`
	Warnings        []string `json:"warnings,omitempty"`
}

type CardIdentifier struct {
	ScryfallID      string
	SetCode         string
	CollectorNumber string
}

type ResolvedCard struct {
	ScryfallID string
	ManaValue  float64
	Colors     string
	CardType   string
}

func (rc ResolvedCard) colorsAndType() (*string, *string) {
	colors, cardType := rc.Colors, rc.CardType
	return &colors, &cardType
}

type DetailsRefreshSummary struct {
	Updated  int `json:"updated"`
	NotFound int `json:"not_found"`
}

type ScryfallResolver interface {
	Resolve(ctx context.Context, identifiers []CardIdentifier) (map[string]ResolvedCard, error)
}

func resolveKey(setCode, collectorNumber string) string {
	return "sc:" + strings.ToLower(setCode) + "#" + collectorNumber
}

func resolveKeyByID(scryfallID string) string {
	return "id:" + strings.ToLower(scryfallID)
}
