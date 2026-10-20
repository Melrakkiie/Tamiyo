package deck

import (
	"context"
	"fmt"
)

// applyRemovals takes the plan's copies and pending cards out of the deck,
// clearing the commander slots they held.
func applyRemovals(ctx context.Context, store CompositionStore, userID string, deckID string, plan PlacementPlan) error {
	if len(plan.RemoveCards) == 0 && len(plan.PendingQuantities) == 0 {
		return nil
	}
	d, err := store.FindByID(ctx, userID, deckID)
	if err != nil {
		return err
	}
	owned, err := store.FindCardsByDeckID(ctx, userID, deckID, "added", false)
	if err != nil {
		return err
	}
	inDeck := make(map[int]bool, len(owned))
	for _, c := range owned {
		inDeck[c.ID] = true
	}

	slotsCleared := false
	for _, id := range plan.RemoveCards {
		if !inDeck[id] {
			return ErrCardNotInDeck
		}
		if slot, ok := d.CardSlot(id); ok {
			d.clearSlot(slot)
			slotsCleared = true
		}
	}
	for _, p := range plan.PendingQuantities {
		if slot, ok := d.PendingSlot(p.ID); ok && p.Quantity <= 0 {
			d.clearSlot(slot)
			slotsCleared = true
		}
	}
	if slotsCleared {
		if _, err := store.Update(ctx, userID, d); err != nil {
			return fmt.Errorf("clearing the commander: %w", err)
		}
	}

	for _, id := range plan.RemoveCards {
		if err := store.UnlinkCardFromDeck(ctx, userID, deckID, id); err != nil {
			return fmt.Errorf("removing a card from the deck: %w", err)
		}
	}
	for _, p := range plan.PendingQuantities {
		if p.Quantity <= 0 {
			err = store.DeletePendingCard(ctx, userID, deckID, p.ID)
		} else {
			_, err = store.UpdatePendingQuantity(ctx, userID, deckID, p.ID, p.Quantity)
		}
		if err != nil {
			return fmt.Errorf("removing a pending card from the deck: %w", err)
		}
	}
	return nil
}
