package deckshare

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/deckinsights"
	"Melrakkiie/Tamiyo/internal/user"
)

var ErrNotFound = errors.New("shared deck not found")

type deckService interface {
	GetDeck(ctx context.Context, userID string, id string) (deck.Deck, error)
	GetSharedDeck(ctx context.Context, deckID string) (string, deck.Deck, error)
	GetDeckContent(ctx context.Context, userID string, deckID string) (deck.DeckContent, error)
	GetCardTags(ctx context.Context, userID string, deckID string) (deck.DeckTags, error)
	BrowsePublicDecks(ctx context.Context, filter deck.PublicFilter) ([]deck.PublicDeck, int, error)
	CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error)
	LikeDeck(ctx context.Context, userID string, deckID string) error
	UnlikeDeck(ctx context.Context, userID string, deckID string) error
	GetLikeStatus(ctx context.Context, userID string, deckID string) (deck.LikeStatus, error)
	GetLikedDecks(ctx context.Context, userID string, page int, limit int) ([]deck.PublicDeck, int, error)
}

type userService interface {
	GetUser(ctx context.Context, userID string) (user.User, error)
}

type insightsService interface {
	GetDeckLegality(ctx context.Context, userID string, deckID string) (deckinsights.LegalityReport, error)
	GetDeckStats(ctx context.Context, userID string, deckID string) (deckinsights.DeckStats, error)
	GetDeckCombos(ctx context.Context, userID string, deckID string) (deckinsights.DeckCombos, error)
}

type Owner struct {
	ID               string
	DisplayName      *string
	AvatarScryfallID *string
}

type Card struct {
	Name            string
	ScryfallID      string
	SetCode         string
	CollectorNumber string
	Foil            bool
	Quantity        int
	ManaValue       float64
	Colors          *string
	CardType        *string
	ColorIdentity   *string
	Commander       bool
	Board           string
	Tags            []string
}

type SharedDeck struct {
	Deck  deck.Deck
	Owner Owner
	Cards []Card
}

type Service struct {
	decks    deckService
	users    userService
	insights insightsService
}

func NewService(decks deckService, users userService, insights insightsService) *Service {
	return &Service{decks: decks, users: users, insights: insights}
}

func (s *Service) resolve(ctx context.Context, deckID string) (string, deck.Deck, error) {
	ownerID, d, err := s.decks.GetSharedDeck(ctx, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return "", deck.Deck{}, ErrNotFound
		}
		return "", deck.Deck{}, err
	}
	return ownerID, d, nil
}

func (s *Service) GetSharedDeck(ctx context.Context, deckID string) (SharedDeck, error) {
	ownerID, d, err := s.resolve(ctx, deckID)
	if err != nil {
		return SharedDeck{}, err
	}

	owner, cards, err := s.loadContent(ctx, ownerID, d)
	if err != nil {
		return SharedDeck{}, err
	}
	return SharedDeck{Deck: d, Owner: owner, Cards: cards}, nil
}

func (s *Service) GetVisibleDeck(ctx context.Context, userID string, deckID string) (SharedDeck, error) {
	ownerID, d, err := s.resolveVisible(ctx, userID, deckID)
	if err != nil {
		return SharedDeck{}, err
	}

	owner, cards, err := s.loadContent(ctx, ownerID, d)
	if err != nil {
		return SharedDeck{}, err
	}
	return SharedDeck{Deck: d, Owner: owner, Cards: cards}, nil
}

func (s *Service) loadContent(ctx context.Context, ownerID string, d deck.Deck) (Owner, []Card, error) {
	owner, err := s.users.GetUser(ctx, ownerID)
	if err != nil {
		return Owner{}, nil, fmt.Errorf("loading deck owner: %w", err)
	}

	content, err := s.decks.GetDeckContent(ctx, ownerID, d.ID)
	if err != nil {
		return Owner{}, nil, fmt.Errorf("loading deck content: %w", err)
	}

	tags, err := s.decks.GetCardTags(ctx, ownerID, d.ID)
	if err != nil {
		return Owner{}, nil, fmt.Errorf("loading card tags: %w", err)
	}

	byName := tags.ByCardName()
	cards := make([]Card, 0, len(content.Entries))
	for _, e := range content.Entries {
		cards = append(cards, sharedCard(e, byName[deck.CardNameKey(e.Name)]))
	}
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].Name < cards[j].Name })

	return Owner{ID: owner.ID, DisplayName: owner.DisplayName, AvatarScryfallID: owner.AvatarScryfallID}, cards, nil
}

func (s *Service) BrowsePublicDecks(ctx context.Context, filter deck.PublicFilter) ([]deck.PublicDeck, int, error) {
	return s.decks.BrowsePublicDecks(ctx, filter)
}

func (s *Service) GetSharedDeckLegality(ctx context.Context, deckID string) (deckinsights.LegalityReport, error) {
	ownerID, d, err := s.resolve(ctx, deckID)
	if err != nil {
		return deckinsights.LegalityReport{}, err
	}
	report, err := s.insights.GetDeckLegality(ctx, ownerID, d.ID)
	if err != nil {
		return deckinsights.LegalityReport{}, err
	}
	for i := range report.Issues {
		report.Issues[i].CardID = 0
	}
	return report, nil
}

func (s *Service) GetSharedDeckStats(ctx context.Context, deckID string) (deckinsights.DeckStats, error) {
	ownerID, d, err := s.resolve(ctx, deckID)
	if err != nil {
		return deckinsights.DeckStats{}, err
	}
	return s.insights.GetDeckStats(ctx, ownerID, d.ID)
}

func (s *Service) GetSharedDeckCombos(ctx context.Context, deckID string) (deckinsights.DeckCombos, error) {
	ownerID, d, err := s.resolve(ctx, deckID)
	if err != nil {
		return deckinsights.DeckCombos{}, err
	}
	return s.insights.GetDeckCombos(ctx, ownerID, d.ID)
}

func sharedCard(e deck.DeckEntry, tags []string) Card {
	return Card{
		Name:            e.Name,
		ScryfallID:      e.ScryfallID,
		SetCode:         e.SetCode,
		CollectorNumber: e.CollectorNumber,
		Foil:            e.Foil,
		Quantity:        e.Quantity(),
		ManaValue:       e.ManaValue,
		Colors:          e.Colors,
		CardType:        e.CardType,
		ColorIdentity:   e.ColorIdentity,
		Commander:       e.IsCommander(),
		Board:           e.Board,
		Tags:            tags,
	}
}
