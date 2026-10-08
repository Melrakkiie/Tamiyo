package bulk

import (
	"context"
	"errors"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
)

var ErrInvalidFile = errors.New("could not parse the uploaded file")

var ErrTargetStorageNotFound = errors.New("target storage does not exist")

var ErrDeckNotFound = errors.New("deck does not exist")

var ErrScryfallUnavailable = errors.New("could not resolve cards against scryfall")

type Summary struct {
	CardsCreated    int      `json:"cards_created"`
	CardsLinked     int      `json:"cards_linked"`
	CardsPending    int      `json:"cards_pending"`
	CardsSkipped    int      `json:"cards_skipped"`
	StoragesCreated int      `json:"storages_created"`
	DecksCreated    int      `json:"decks_created"`
	Warnings        []string `json:"warnings,omitempty"`
}

type CardIdentifier struct {
	ScryfallID      string
	SetCode         string
	CollectorNumber string
	Name            string
}

type ResolvedCard struct {
	ScryfallID      string
	Name            string
	SetCode         string
	CollectorNumber string
	ManaValue       float64
	Colors          string
	CardType        string
	ColorIdentity   string
}

func (rc ResolvedCard) details() (*string, *string, *string) {
	colors, cardType, identity := rc.Colors, rc.CardType, rc.ColorIdentity
	return &colors, &cardType, &identity
}

type DetailsRefreshSummary struct {
	Updated     int  `json:"updated"`
	NotFound    int  `json:"not_found"`
	Remaining   int  `json:"remaining"`
	NextAfterID *int `json:"next_after_id"`
}

type ScryfallResolver interface {
	Resolve(ctx context.Context, identifiers []CardIdentifier) (map[string]ResolvedCard, error)
}

func resolveKey(setCode, collectorNumber string) string {
	return "sc:" + strings.ToLower(setCode) + "#" + collectorNumber
}

func resolveKeyByName(name string) string {
	return "name:" + deck.CardNameKey(name)
}

func frontFaceName(name string) string {
	front, _, _ := strings.Cut(strings.ReplaceAll(name, "//", "/"), "/")
	return strings.TrimSpace(front)
}

func resolveKeyByID(scryfallID string) string {
	return "id:" + strings.ToLower(scryfallID)
}
