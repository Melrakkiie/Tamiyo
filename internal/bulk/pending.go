package bulk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

type PendingCommitSummary struct {
	CardsCreated int `json:"cards_created"`
}

func (s *Service) CommitPendingCards(ctx context.Context, userID string, deckID string, storageID, pendingID, quantity *int) (PendingCommitSummary, error) {
	var summary PendingCommitSummary

	pending, err := s.decks.GetPendingCards(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return summary, ErrDeckNotFound
		}
		return summary, err
	}

	if pendingID != nil {
		pending = onlyPendingCard(pending, *pendingID)
		if len(pending) == 0 {
			return summary, deck.ErrPendingCardNotFound
		}
	}

	if storageID != nil {
		if _, err := s.storages.GetStorage(ctx, userID, *storageID); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return summary, ErrTargetStorageNotFound
			}
			return summary, err
		}
	}

	for _, p := range pending {
		count := p.Quantity
		if quantity != nil && *quantity < count {
			count = *quantity
		}
		for i := 0; i < count; i++ {
			created, err := s.cards.CreateCard(ctx, userID, card.Card{
				Name:            p.Name,
				ScryfallID:      p.ScryfallID,
				SetCode:         p.SetCode,
				CollectorNumber: p.CollectorNumber,
				Foil:            p.Foil,
				StorageID:       storageID,
				ManaValue:       p.ManaValue,
				Colors:          p.Colors,
				CardType:        p.CardType,
				ColorIdentity:   p.ColorIdentity,
			})
			if err != nil {
				return summary, fmt.Errorf("creating %q: %w", p.Name, err)
			}
			summary.CardsCreated++
			if err := s.decks.PutCardInDeck(ctx, userID, deckID, created.ID); err != nil {
				return summary, fmt.Errorf("adding %q to the deck: %w", p.Name, err)
			}
			if i == 0 {
				if err := s.decks.PromotePendingCommander(ctx, userID, deckID, p.ID, created.ID); err != nil {
					return summary, fmt.Errorf("making %q the commander: %w", p.Name, err)
				}
			}
		}
		if count < p.Quantity {
			if _, err := s.decks.SetPendingQuantity(ctx, userID, deckID, p.ID, p.Quantity-count); err != nil {
				return summary, fmt.Errorf("updating %q in the pending list: %w", p.Name, err)
			}
			continue
		}
		if err := s.decks.RemovePendingCard(ctx, userID, deckID, p.ID); err != nil {
			return summary, fmt.Errorf("clearing %q from the pending list: %w", p.Name, err)
		}
	}

	return summary, nil
}

func onlyPendingCard(pending []deck.PendingCard, id int) []deck.PendingCard {
	for _, p := range pending {
		if p.ID == id {
			return []deck.PendingCard{p}
		}
	}
	return nil
}

type commitPendingRequest struct {
	StorageID *int `json:"storage_id"`
	PendingID *int `json:"pending_id"`
	Quantity  *int `json:"quantity"`
}

func (h *Handler) commitPendingCards(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	deckID, valid := deck.ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req commitPendingRequest
	if ctx.Request.ContentLength != 0 {
		if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		if req.StorageID != nil && *req.StorageID <= 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id must be a positive integer"})
			return
		}
		if req.PendingID != nil && *req.PendingID <= 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "pending_id must be a positive integer"})
			return
		}
		if req.Quantity != nil && (req.PendingID == nil || *req.Quantity <= 0) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "quantity must be a positive integer and needs a pending_id"})
			return
		}
	}

	summary, err := h.service.CommitPendingCards(ctx.Request.Context(), userID, deckID, req.StorageID, req.PendingID, req.Quantity)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: deck.ErrPendingCardNotFound, Status: http.StatusNotFound, Message: "pending card not found"},
			apierr.Mapping{Err: ErrTargetStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"},
		)
		return
	}

	ctx.JSON(http.StatusOK, summary)
}
