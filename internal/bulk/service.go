package bulk

import (
	"context"
	"errors"
	"fmt"
	"io"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

const defaultManaBoxDeckFormat = "commander"

type cardService interface {
	CreateCard(ctx context.Context, userID string, c card.Card) (card.Card, error)
	GetAllCards(ctx context.Context, userID string, filter card.CardFilter) ([]card.Card, int, error)
	GetCardsMissingDetails(ctx context.Context, userID string, afterID int, limit int) ([]card.Card, error)
	CountCardsMissingDetails(ctx context.Context, userID string, afterID int) (int, error)
	SetCardDetails(ctx context.Context, userID string, id int, details card.Details) error
}

type storageService interface {
	GetAllStorages(ctx context.Context, userID string, filter storage.Filter) ([]storage.Storage, int, error)
	GetStorage(ctx context.Context, userID string, id int) (storage.Storage, error)
	CreateStorage(ctx context.Context, userID string, s storage.Storage) (storage.Storage, error)
}

type deckService interface {
	GetAllDecks(ctx context.Context, userID string, filter deck.Filter) ([]deck.Deck, int, error)
	GetDeck(ctx context.Context, userID string, id string) (deck.Deck, error)
	CreateDeck(ctx context.Context, userID string, d deck.Deck) (deck.Deck, error)
	PutCardInDeck(ctx context.Context, userID string, deckID string, cardID int) error
	GetDeckCards(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]deck.DeckCard, error)
	GetPendingCards(ctx context.Context, userID string, deckID string) ([]deck.PendingCard, error)
	RemovePendingCard(ctx context.Context, userID string, deckID string, id int) error
	PromotePendingCommander(ctx context.Context, userID string, deckID string, pendingID, cardID int) error
	AddPendingCard(ctx context.Context, userID string, deckID string, p deck.PendingCard) (deck.PendingCard, error)
	SetPendingCommander(ctx context.Context, userID string, deckID string, pendingID int) error
	SetCardCommander(ctx context.Context, userID string, deckID string, cardID int) error
}

type Service struct {
	cards    cardService
	storages storageService
	decks    deckService
	scryfall ScryfallResolver
}

func NewService(cards cardService, storages storageService, decks deckService, scryfall ScryfallResolver) *Service {
	return &Service{cards: cards, storages: storages, decks: decks, scryfall: scryfall}
}

type MoxfieldDeckImportRequest struct {
	Name   string
	Format string

	CommanderFromFirstLine bool
}

func (s *Service) ImportManaBox(ctx context.Context, userID string, r io.Reader) (Summary, error) {
	rows, err := parseManaBoxCSV(r)
	if err != nil {
		return Summary{}, err
	}

	storageCache, err := s.loadStorageCache(ctx, userID)
	if err != nil {
		return Summary{}, fmt.Errorf("loading existing storages: %w", err)
	}
	deckCache, err := s.loadDeckCache(ctx, userID)
	if err != nil {
		return Summary{}, fmt.Errorf("loading existing decks: %w", err)
	}

	resolvedByScryfallID := make(map[string]ResolvedCard)
	identifiers := make([]CardIdentifier, 0, len(rows))
	for _, row := range rows {
		if row.ScryfallID != "" {
			identifiers = append(identifiers, CardIdentifier{ScryfallID: row.ScryfallID})
		}
	}
	resolved, resolveErr := s.scryfall.Resolve(ctx, dedupeIdentifiers(identifiers))
	if resolveErr != nil {
		resolved = map[string]ResolvedCard{}
	}
	for _, rc := range resolved {
		resolvedByScryfallID[rc.ScryfallID] = rc
	}

	var summary Summary
	if resolveErr != nil {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not resolve mana values against scryfall, imported cards default to mana_value 0: %v", resolveErr))
	}

	for _, row := range rows {
		storageID, created, err := s.getOrCreateStorage(ctx, userID, storageCache, row.BinderName, row.BinderType)
		if err != nil {
			return summary, fmt.Errorf("line %d: creating storage %q: %w", row.LineNo, row.BinderName, err)
		}
		if created {
			summary.StoragesCreated++
		}

		var deckID *string
		if row.BinderType == "deck" {
			id, created, err := s.getOrCreateDeck(ctx, userID, deckCache, row.BinderName, defaultManaBoxDeckFormat)
			if err != nil {
				return summary, fmt.Errorf("line %d: creating deck %q: %w", row.LineNo, row.BinderName, err)
			}
			if created {
				summary.DecksCreated++
			}
			deckID = &id
		}

		rc, resolvedOK := resolvedByScryfallID[row.ScryfallID]
		var colors, cardType, identity *string
		if resolvedOK {
			colors, cardType, identity = rc.details()
		}

		for i := 0; i < row.Quantity; i++ {
			created, err := s.cards.CreateCard(ctx, userID, card.Card{
				Name:            row.CardName,
				ScryfallID:      row.ScryfallID,
				SetCode:         row.SetCode,
				CollectorNumber: row.CollectorNumber,
				Foil:            row.Foil,
				StorageID:       &storageID,
				ManaValue:       rc.ManaValue,
				Colors:          colors,
				CardType:        cardType,
				ColorIdentity:   identity,
			})
			if err != nil {
				summary.CardsSkipped++
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: could not create %q: %v", row.LineNo, row.CardName, err))
				continue
			}
			summary.CardsCreated++

			if deckID != nil {
				if err := s.decks.PutCardInDeck(ctx, userID, *deckID, created.ID); err != nil {
					summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: could not link %q to deck: %v", row.LineNo, row.CardName, err))
				}
			}
		}
	}

	return summary, nil
}

func (s *Service) ImportMoxfieldCollection(ctx context.Context, userID string, storageID int, r io.Reader) (Summary, error) {
	rows, err := parseMoxfieldCollectionCSV(r)
	if err != nil {
		return Summary{}, err
	}

	if _, err := s.storages.GetStorage(ctx, userID, storageID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return Summary{}, ErrTargetStorageNotFound
		}
		return Summary{}, err
	}

	identifiers := make([]CardIdentifier, len(rows))
	for i, row := range rows {
		identifiers[i] = CardIdentifier{SetCode: row.SetCode, CollectorNumber: row.CollectorNumber}
	}
	resolved, err := s.scryfall.Resolve(ctx, dedupeIdentifiers(identifiers))
	if err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
	}

	var summary Summary
	for _, row := range rows {
		resolvedCard, ok := resolved[resolveKey(row.SetCode, row.CollectorNumber)]
		if !ok {
			summary.CardsSkipped += row.Quantity
			summary.Warnings = append(summary.Warnings, fmt.Sprintf(
				"line %d: %q (%s #%s) not found on scryfall", row.LineNo, row.CardName, row.SetCode, row.CollectorNumber,
			))
			continue
		}

		colors, cardType, identity := resolvedCard.details()
		for i := 0; i < row.Quantity; i++ {
			if _, err := s.cards.CreateCard(ctx, userID, card.Card{
				Name:            row.CardName,
				ScryfallID:      resolvedCard.ScryfallID,
				SetCode:         row.SetCode,
				CollectorNumber: row.CollectorNumber,
				Foil:            row.Foil,
				StorageID:       &storageID,
				ManaValue:       resolvedCard.ManaValue,
				Colors:          colors,
				CardType:        cardType,
				ColorIdentity:   identity,
			}); err != nil {
				summary.CardsSkipped++
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: could not create %q: %v", row.LineNo, row.CardName, err))
				continue
			}
			summary.CardsCreated++
		}
	}

	return summary, nil
}

func (s *Service) loadAllCards(ctx context.Context, userID string) ([]card.Card, error) {
	var all []card.Card
	const limit = 100
	for page := 1; ; page++ {
		items, total, err := s.cards.GetAllCards(ctx, userID, card.CardFilter{Page: page, Limit: limit})
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) == 0 || page*limit >= total {
			return all, nil
		}
	}
}

func (s *Service) loadAllStoragesByID(ctx context.Context, userID string) (map[int]storage.Storage, error) {
	byID := make(map[int]storage.Storage)
	const limit = 100
	for page := 1; ; page++ {
		items, total, err := s.storages.GetAllStorages(ctx, userID, storage.Filter{Page: page, Limit: limit})
		if err != nil {
			return nil, err
		}
		for _, st := range items {
			byID[st.ID] = st
		}
		if len(items) == 0 || page*limit >= total {
			return byID, nil
		}
	}
}

func (s *Service) loadStorageCache(ctx context.Context, userID string) (map[string]int, error) {
	cache := make(map[string]int)
	const limit = 100
	for page := 1; ; page++ {
		items, total, err := s.storages.GetAllStorages(ctx, userID, storage.Filter{Page: page, Limit: limit})
		if err != nil {
			return nil, err
		}
		for _, st := range items {
			cache[st.Name] = st.ID
		}
		if len(items) == 0 || page*limit >= total {
			return cache, nil
		}
	}
}

func (s *Service) loadDeckCache(ctx context.Context, userID string) (map[string]string, error) {
	cache := make(map[string]string)
	const limit = 100
	for page := 1; ; page++ {
		items, total, err := s.decks.GetAllDecks(ctx, userID, deck.Filter{Page: page, Limit: limit})
		if err != nil {
			return nil, err
		}
		for _, d := range items {
			cache[d.Name] = d.ID
		}
		if len(items) == 0 || page*limit >= total {
			return cache, nil
		}
	}
}

func (s *Service) getOrCreateStorage(ctx context.Context, userID string, cache map[string]int, name, storageType string) (id int, created bool, err error) {
	if id, ok := cache[name]; ok {
		return id, false, nil
	}
	st, err := s.storages.CreateStorage(ctx, userID, storage.Storage{Name: name, Type: storageType})
	if err != nil {
		return 0, false, err
	}
	cache[name] = st.ID
	return st.ID, true, nil
}

func (s *Service) getOrCreateDeck(ctx context.Context, userID string, cache map[string]string, name, format string) (id string, created bool, err error) {
	if id, ok := cache[name]; ok {
		return id, false, nil
	}
	d, err := s.decks.CreateDeck(ctx, userID, deck.Deck{Name: name, Format: format})
	if err != nil {
		return "", false, err
	}
	cache[name] = d.ID
	return d.ID, true, nil
}

func identifierKey(id CardIdentifier) string {
	if id.ScryfallID != "" {
		return resolveKeyByID(id.ScryfallID)
	}
	if id.SetCode == "" && id.Name != "" {
		return resolveKeyByName(id.Name)
	}
	return resolveKey(id.SetCode, id.CollectorNumber)
}

func dedupeIdentifiers(identifiers []CardIdentifier) []CardIdentifier {
	seen := make(map[string]struct{}, len(identifiers))
	out := make([]CardIdentifier, 0, len(identifiers))
	for _, id := range identifiers {
		key := identifierKey(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}
	return out
}

const RefreshDetailsChunkSize = 750

func (s *Service) RefreshCardDetails(ctx context.Context, userID string, afterID int) (DetailsRefreshSummary, error) {
	cards, err := s.cards.GetCardsMissingDetails(ctx, userID, afterID, RefreshDetailsChunkSize)
	if err != nil {
		return DetailsRefreshSummary{}, err
	}
	if len(cards) == 0 {
		return DetailsRefreshSummary{}, nil
	}

	identifiers := make([]CardIdentifier, 0, len(cards))
	for _, c := range cards {
		identifiers = append(identifiers, CardIdentifier{ScryfallID: c.ScryfallID})
	}
	resolved, err := s.scryfall.Resolve(ctx, dedupeIdentifiers(identifiers))
	if err != nil {
		return DetailsRefreshSummary{}, fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
	}

	var summary DetailsRefreshSummary
	for _, c := range cards {
		rc, ok := resolved[resolveKeyByID(c.ScryfallID)]
		if !ok {
			summary.NotFound++
			continue
		}
		details := card.Details{Colors: rc.Colors, CardType: rc.CardType, ColorIdentity: rc.ColorIdentity, ManaValue: rc.ManaValue}
		if err := s.cards.SetCardDetails(ctx, userID, c.ID, details); err != nil {
			return summary, err
		}
		summary.Updated++
	}

	lastID := cards[len(cards)-1].ID
	remaining, err := s.cards.CountCardsMissingDetails(ctx, userID, lastID)
	if err != nil {
		return summary, err
	}
	summary.Remaining = remaining
	if remaining > 0 {
		summary.NextAfterID = &lastID
	}

	return summary, nil
}
