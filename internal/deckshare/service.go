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
	GetDeckCards(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]deck.DeckCard, error)
	GetPendingCards(ctx context.Context, userID string, deckID string) ([]deck.PendingCard, error)
	GetCardTags(ctx context.Context, userID string, deckID string) (deck.DeckTags, error)
}

type userService interface {
	GetUser(ctx context.Context, userID string) (user.User, error)
}

type insightsService interface {
	GetDeckLegality(ctx context.Context, userID string, deckID string) (deckinsights.LegalityReport, error)
	GetDeckStats(ctx context.Context, userID string, deckID string) (deckinsights.DeckStats, error)
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

func (s *Service) loadContent(ctx context.Context, ownerID string, d deck.Deck) (Owner, []Card, error) {
	owner, err := s.users.GetUser(ctx, ownerID)
	if err != nil {
		return Owner{}, nil, fmt.Errorf("loading deck owner: %w", err)
	}

	owned, err := s.decks.GetDeckCards(ctx, ownerID, d.ID, "name", false)
	if err != nil {
		return Owner{}, nil, fmt.Errorf("loading deck cards: %w", err)
	}

	pending, err := s.decks.GetPendingCards(ctx, ownerID, d.ID)
	if err != nil {
		return Owner{}, nil, fmt.Errorf("loading pending cards: %w", err)
	}

	tags, err := s.decks.GetCardTags(ctx, ownerID, d.ID)
	if err != nil {
		return Owner{}, nil, fmt.Errorf("loading card tags: %w", err)
	}

	cards := mergeCards(d, owned, pending)
	byName := tags.ByCardName()
	for i := range cards {
		cards[i].Tags = byName[deck.CardNameKey(cards[i].Name)]
	}

	return Owner{ID: owner.ID, DisplayName: owner.DisplayName, AvatarScryfallID: owner.AvatarScryfallID}, cards, nil
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

type cardKey struct {
	scryfallID string
	foil       bool
	commander  bool
}

func mergeCards(d deck.Deck, owned []deck.DeckCard, pending []deck.PendingCard) []Card {
	merged := make(map[cardKey]*Card)
	var order []cardKey

	add := func(key cardKey, c Card) {
		if existing, ok := merged[key]; ok {
			existing.Quantity += c.Quantity
			return
		}
		merged[key] = &c
		order = append(order, key)
	}

	for _, c := range owned {
		commander := d.CommanderID != nil && *d.CommanderID == c.ID
		add(cardKey{c.ScryfallID, c.Foil, commander}, Card{
			Name:            c.Name,
			ScryfallID:      c.ScryfallID,
			SetCode:         c.SetCode,
			CollectorNumber: c.CollectorNumber,
			Foil:            c.Foil,
			Quantity:        1,
			ManaValue:       c.ManaValue,
			Colors:          c.Colors,
			CardType:        c.CardType,
			ColorIdentity:   c.ColorIdentity,
			Commander:       commander,
		})
	}

	for _, p := range pending {
		quantity := p.Quantity
		if quantity < 1 {
			quantity = 1
		}
		commander := d.CommanderPendingID != nil && *d.CommanderPendingID == p.ID
		if commander {
			add(cardKey{p.ScryfallID, p.Foil, true}, pendingCard(p, 1, true))
			quantity--
		}
		if quantity > 0 {
			add(cardKey{p.ScryfallID, p.Foil, false}, pendingCard(p, quantity, false))
		}
	}

	cards := make([]Card, 0, len(order))
	for _, key := range order {
		cards = append(cards, *merged[key])
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].Name != cards[j].Name {
			return cards[i].Name < cards[j].Name
		}
		return cards[i].SetCode < cards[j].SetCode
	})
	return cards
}

func pendingCard(p deck.PendingCard, quantity int, commander bool) Card {
	return Card{
		Name:            p.Name,
		ScryfallID:      p.ScryfallID,
		SetCode:         p.SetCode,
		CollectorNumber: p.CollectorNumber,
		Foil:            p.Foil,
		Quantity:        quantity,
		ManaValue:       p.ManaValue,
		Colors:          p.Colors,
		CardType:        p.CardType,
		ColorIdentity:   p.ColorIdentity,
		Commander:       commander,
	}
}
