package bulk

import (
	"context"
	"fmt"

	"Melrakkiie/Tamiyo/internal/deck"
)

func (s *Service) commitEntriesPending(ctx context.Context, userID string, deckID string, storageID *int, entries []deck.DeckEntry, summary *Summary) {
	for _, p := range deck.PendingTotals(entries) {
		pendingID := p.ID
		result, err := s.decks.CommitPendingCards(ctx, userID, deckID, deck.PendingCommit{StorageID: storageID, PendingID: &pendingID})
		summary.CardsCreated += result.CardsCreated
		summary.CardsLinked += result.CardsCreated
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%q: %v", p.Name, err))
		}
	}
}
