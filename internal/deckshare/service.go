package deckshare

import (
	"context"
	"fmt"
	"sort"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/user"
)

type deckService interface {
	ReadDeck(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (deck.ReadDeck, error)
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
	GameChanger     bool
	Board           string
	Tags            []string
}

type SharedDeck struct {
	Deck  deck.Deck
	Owner Owner
	Cards []Card
}

type Service struct {
	decks deckService
	users userService
}

func NewService(decks deckService, users userService) *Service {
	return &Service{decks: decks, users: users}
}

func (s *Service) GetSharedDeck(ctx context.Context, deckID string) (SharedDeck, error) {
	read, err := s.decks.ReadDeck(ctx, "", deckID, deck.ReachShared)
	if err != nil {
		return SharedDeck{}, err
	}
	return s.sharedDeck(ctx, read)
}

func (s *Service) sharedDeck(ctx context.Context, read deck.ReadDeck) (SharedDeck, error) {
	owner, err := s.owner(ctx, read.OwnerID)
	if err != nil {
		return SharedDeck{}, err
	}
	return SharedDeck{Deck: read.Deck, Owner: owner, Cards: sharedCards(read.Entries, read.Tags)}, nil
}

func (s *Service) owner(ctx context.Context, ownerID string) (Owner, error) {
	owner, err := s.users.GetUser(ctx, ownerID)
	if err != nil {
		return Owner{}, fmt.Errorf("loading deck owner: %w", err)
	}
	return Owner{ID: owner.ID, DisplayName: owner.DisplayName, AvatarScryfallID: owner.AvatarScryfallID}, nil
}

func sharedCards(entries []deck.DeckEntry, tagsByName map[string][]string) []Card {
	cards := make([]Card, 0, len(entries))
	for _, e := range entries {
		cards = append(cards, sharedCard(e, tagsByName[deck.CardNameKey(e.Name)]))
	}
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].Name < cards[j].Name })
	return cards
}

func (s *Service) GetVisibleDeck(ctx context.Context, userID string, deckID string) (SharedDeck, error) {
	read, err := s.decks.ReadDeck(ctx, userID, deckID, deck.ReachVisible)
	if err != nil {
		return SharedDeck{}, err
	}
	return s.sharedDeck(ctx, read)
}

func (s *Service) BrowsePublicDecks(ctx context.Context, filter deck.PublicFilter) ([]deck.PublicDeck, int, error) {
	return s.decks.BrowsePublicDecks(ctx, filter)
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
		GameChanger:     e.GameChanger,
		Board:           e.Board,
		Tags:            tags,
	}
}
