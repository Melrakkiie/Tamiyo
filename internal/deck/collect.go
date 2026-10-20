package deck

import (
	"context"
	"fmt"
	"slices"
)

// OwnDeckCollect picks what CollectOwnDeck adds to the collection from the
// user's own deck: the pending cards of Boards and, with OwnedCopies, one
// more copy of each owned card of Boards, into StorageID when given.
type OwnDeckCollect struct {
	Boards      []string
	StorageID   *int
	OwnedCopies bool
}

// CollectSummary counts the collection cards a collect created, and among
// them those put in the deck.
type CollectSummary struct {
	CardsCreated int
	CardsLinked  int
}

// CollectOwnDeck commits the pending cards of the chosen boards as
// CommitPendingCards does and, with OwnedCopies, creates a copy of each owned
// card of those boards that stays out of any deck, all in one transaction.
func (s *Service) CollectOwnDeck(ctx context.Context, userID string, deckID string, req OwnDeckCollect) (CollectSummary, error) {
	var summary CollectSummary
	err := s.repo.Compose(ctx, userID, deckID, func(store CompositionStore) error {
		summary = CollectSummary{}
		if err := requireStorage(ctx, store, userID, req.StorageID); err != nil {
			return err
		}
		content, err := readDeckContent(ctx, store, userID, deckID)
		if err != nil {
			return err
		}
		entries := EntriesInBoards(content.Entries, req.Boards)
		pending, err := store.FindPendingCards(ctx, userID, deckID)
		if err != nil {
			return err
		}
		if req.OwnedCopies {
			for _, e := range entries {
				if err := createEntryCopies(ctx, store, userID, e, e.OwnedQuantity(), req.StorageID); err != nil {
					return err
				}
				summary.CardsCreated += e.OwnedQuantity()
			}
		}
		d := content.Deck
		for _, total := range PendingTotals(entries) {
			p, found := findPending(pending, total.ID)
			if !found {
				return ErrPendingCardNotFound
			}
			if d, err = commitPendingCard(ctx, store, userID, d, p, total.Quantity, req.StorageID); err != nil {
				return err
			}
			summary.CardsCreated += total.Quantity
			summary.CardsLinked += total.Quantity
		}
		return nil
	})
	if err != nil {
		return CollectSummary{}, err
	}
	return summary, nil
}

// SharedDeckCollect picks what CollectSharedDeck adds to the collection from
// someone else's shared deck: every copy of Boards, owned or pending, or with
// MissingOnly those the user lacks by card name, into StorageID when given.
type SharedDeckCollect struct {
	Boards      []string
	StorageID   *int
	MissingOnly bool
}

// CollectSharedDeck creates in the user's collection the cards of the chosen
// boards of a public or unlisted deck, in one transaction. The deck is left
// alone.
func (s *Service) CollectSharedDeck(ctx context.Context, userID string, deckID string, req SharedDeckCollect) (CollectSummary, error) {
	ownerID, _, err := s.repo.FindShared(ctx, deckID)
	if err != nil {
		return CollectSummary{}, err
	}
	var summary CollectSummary
	err = s.repo.ComposeCollection(ctx, func(store CompositionStore) error {
		summary = CollectSummary{}
		if err := requireStorage(ctx, store, userID, req.StorageID); err != nil {
			return err
		}
		content, err := readDeckContent(ctx, store, ownerID, deckID)
		if err != nil {
			return err
		}
		entries := EntriesInBoards(content.Entries, req.Boards)
		copies := make([]int, len(entries))
		for i, e := range entries {
			copies[i] = e.Quantity()
		}
		if req.MissingOnly {
			if err := withoutOwnedCopies(ctx, store, userID, entries, copies); err != nil {
				return err
			}
		}
		for i, e := range entries {
			if err := createEntryCopies(ctx, store, userID, e, copies[i], req.StorageID); err != nil {
				return err
			}
			summary.CardsCreated += copies[i]
		}
		return nil
	})
	if err != nil {
		return CollectSummary{}, err
	}
	return summary, nil
}

// withoutOwnedCopies lowers copies[i], the copies wanted of entries[i], by the
// copies of its card name the user already owns in any printing, each owned
// copy covering one wanted copy.
func withoutOwnedCopies(ctx context.Context, store CompositionStore, userID string, entries []DeckEntry, copies []int) error {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		if key := CardNameKey(e.Name); !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	owned, err := store.CountCopiesByName(ctx, userID, keys)
	if err != nil {
		return fmt.Errorf("counting owned copies: %w", err)
	}
	for i, e := range entries {
		key := CardNameKey(e.Name)
		covered := min(owned[key], copies[i])
		owned[key] -= covered
		copies[i] -= covered
	}
	return nil
}

func createEntryCopies(ctx context.Context, store CompositionStore, userID string, e DeckEntry, count int, storageID *int) error {
	for range count {
		if _, err := store.CreateCollectionCard(ctx, userID, entryCopy(e, storageID)); err != nil {
			return fmt.Errorf("creating %q: %w", e.Name, err)
		}
	}
	return nil
}

func entryCopy(e DeckEntry, storageID *int) DeckCard {
	return DeckCard{
		Name:            e.Name,
		ScryfallID:      e.ScryfallID,
		SetCode:         e.SetCode,
		CollectorNumber: e.CollectorNumber,
		Foil:            e.Foil,
		Proxy:           e.Proxy,
		StorageID:       storageID,
		ManaValue:       e.ManaValue,
		Colors:          e.Colors,
		CardType:        e.CardType,
		ColorIdentity:   e.ColorIdentity,
	}
}

func requireStorage(ctx context.Context, store CompositionStore, userID string, storageID *int) error {
	if storageID == nil {
		return nil
	}
	exists, err := store.StorageExists(ctx, userID, *storageID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrTargetStorageNotFound
	}
	return nil
}

// EntriesInBoards keeps the entries on one of the boards.
func EntriesInBoards(entries []DeckEntry, boards []string) []DeckEntry {
	kept := make([]DeckEntry, 0, len(entries))
	for _, e := range entries {
		if slices.Contains(boards, BoardOrMain(e.Board)) {
			kept = append(kept, e)
		}
	}
	return kept
}
