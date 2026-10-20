package deck

import (
	"context"
	"fmt"
)

// applyRemovals takes the plan's copies and pending cards out of the deck,
// clearing the commander slots they held.
func applyRemovals(ctx context.Context, store CompositionStore, userID string, deckID string, plan PlacementPlan) error {
	if len(plan.RemoveCards) == 0 && len(plan.PendingRemovals) == 0 {
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

	pending, err := store.FindPendingCards(ctx, userID, deckID)
	if err != nil {
		return err
	}
	held := make(map[int]int, len(pending))
	for _, p := range pending {
		held[p.ID] = p.Quantity
	}
	remaining := make(map[int]int, len(plan.PendingRemovals))
	var touched []int
	for _, p := range plan.PendingRemovals {
		current, ok := remaining[p.ID]
		if !ok {
			current = held[p.ID]
			touched = append(touched, p.ID)
		}
		if p.Count > current {
			return ErrCardNotInDeck
		}
		remaining[p.ID] = current - p.Count
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
	for _, id := range touched {
		left := remaining[id]
		if slot, ok := d.PendingSlot(id); ok && left == 0 {
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
	for _, id := range touched {
		left := remaining[id]
		if left == 0 {
			err = store.DeletePendingCard(ctx, userID, deckID, id)
		} else {
			_, err = store.UpdatePendingQuantity(ctx, userID, deckID, id, left)
		}
		if err != nil {
			return fmt.Errorf("removing a pending card from the deck: %w", err)
		}
	}
	return nil
}
