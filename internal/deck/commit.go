package deck

import (
	"context"
	"errors"
	"fmt"
)

var ErrTargetStorageNotFound = errors.New("target storage does not exist")

// PendingCommit picks what CommitPendingCards adds to the collection: every
// pending card off the considering board, or only PendingID (Quantity copies
// of it when given), into StorageID when given.
type PendingCommit struct {
	StorageID *int
	PendingID *int
	Quantity  *int
}

type PendingCommitSummary struct {
	CardsCreated int
}

// CommitPendingCards creates the chosen pending cards in the collection, puts
// each copy in the deck on its pending card's board, moves a pending
// commander or second commander slot to its first copy, then lowers or
// removes the pending card (ADR-0004).
func (s *Service) CommitPendingCards(ctx context.Context, userID string, deckID string, req PendingCommit) (PendingCommitSummary, error) {
	var summary PendingCommitSummary
	err := s.repo.Compose(ctx, userID, deckID, func(store CompositionStore) error {
		summary = PendingCommitSummary{}
		if req.StorageID != nil {
			exists, err := store.StorageExists(ctx, userID, *req.StorageID)
			if err != nil {
				return err
			}
			if !exists {
				return ErrTargetStorageNotFound
			}
		}
		d, err := store.FindByID(ctx, userID, deckID)
		if err != nil {
			return err
		}
		pending, err := store.FindPendingCards(ctx, userID, deckID)
		if err != nil {
			return err
		}
		chosen, err := pendingToCommit(pending, req.PendingID)
		if err != nil {
			return err
		}
		for _, p := range chosen {
			count := p.Quantity
			if req.Quantity != nil && *req.Quantity < count {
				count = *req.Quantity
			}
			if d, err = commitPendingCard(ctx, store, userID, d, p, count, req.StorageID); err != nil {
				return err
			}
			summary.CardsCreated += count
		}
		return nil
	})
	if err != nil {
		return PendingCommitSummary{}, err
	}
	return summary, nil
}

func commitPendingCard(ctx context.Context, store CompositionStore, userID string, d Deck, p PendingCard, count int, storageID *int) (Deck, error) {
	for i := 0; i < count; i++ {
		cardID, err := store.CreateCollectionCard(ctx, userID, pendingCopy(p, storageID))
		if err != nil {
			return d, fmt.Errorf("creating %q: %w", p.Name, err)
		}
		if err := store.LinkCardToDeck(ctx, userID, d.ID, cardID, p.Board); err != nil {
			return d, fmt.Errorf("adding %q to the deck: %w", p.Name, err)
		}
		if slot, ok := d.PendingSlot(p.ID); ok && i == 0 {
			d.setSlot(slot, &cardID, nil)
			if d, err = store.Update(ctx, userID, d); err != nil {
				return d, fmt.Errorf("making %q the commander: %w", p.Name, err)
			}
		}
	}
	if count < p.Quantity {
		if _, err := store.UpdatePendingQuantity(ctx, userID, d.ID, p.ID, p.Quantity-count); err != nil {
			return d, fmt.Errorf("updating %q in the pending list: %w", p.Name, err)
		}
		return d, nil
	}
	if err := store.DeletePendingCard(ctx, userID, d.ID, p.ID); err != nil {
		return d, fmt.Errorf("clearing %q from the pending list: %w", p.Name, err)
	}
	return d, nil
}

func pendingToCommit(pending []PendingCard, pendingID *int) ([]PendingCard, error) {
	if pendingID != nil {
		p, found := findPending(pending, *pendingID)
		if !found {
			return nil, ErrPendingCardNotFound
		}
		return []PendingCard{p}, nil
	}
	kept := make([]PendingCard, 0, len(pending))
	for _, p := range pending {
		if p.Board != BoardConsidering {
			kept = append(kept, p)
		}
	}
	return kept, nil
}

func pendingCopy(p PendingCard, storageID *int) DeckCard {
	return DeckCard{
		Name:            p.Name,
		ScryfallID:      p.ScryfallID,
		SetCode:         p.SetCode,
		CollectorNumber: p.CollectorNumber,
		Foil:            p.Foil,
		Proxy:           p.Proxy,
		StorageID:       storageID,
		ManaValue:       p.ManaValue,
		Colors:          p.Colors,
		CardType:        p.CardType,
		ColorIdentity:   p.ColorIdentity,
	}
}
