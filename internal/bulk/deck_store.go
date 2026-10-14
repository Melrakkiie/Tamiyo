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
)

type StoreRequest struct {
	StorageID *int     `json:"storage_id"`
	Boards    []string `json:"boards"`
}

type StoreSummary struct {
	CardsMoved int `json:"cards_moved"`
}

func (s *Service) StoreDeck(ctx context.Context, userID string, deckID string, storageID int, boards []string) (StoreSummary, error) {
	if _, err := s.decks.GetDeck(ctx, userID, deckID); err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return StoreSummary{}, ErrDeckNotFound
		}
		return StoreSummary{}, err
	}
	wanted, err := wantedBoards(boards)
	if err != nil {
		return StoreSummary{}, err
	}
	if err := s.checkStorage(ctx, userID, storageID); err != nil {
		return StoreSummary{}, err
	}

	cards, err := s.decks.GetDeckCards(ctx, userID, deckID, "name", false)
	if err != nil {
		return StoreSummary{}, fmt.Errorf("loading deck cards: %w", err)
	}
	var ids []int
	for _, c := range cards {
		if !wanted[boardOf(c.Board)] || (c.StorageID != nil && *c.StorageID == storageID) {
			continue
		}
		ids = append(ids, c.ID)
	}

	moved, err := s.cards.MoveCardsToStorage(ctx, userID, ids, storageID)
	if err != nil {
		if errors.Is(err, card.ErrStorageNotFound) {
			return StoreSummary{}, ErrTargetStorageNotFound
		}
		return StoreSummary{}, err
	}
	return StoreSummary{CardsMoved: moved}, nil
}

func (h *Handler) storeDeck(ctx *gin.Context) {
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

	var req StoreRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.StorageID == nil || *req.StorageID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id is required and must be a positive integer"})
		return
	}

	summary, err := h.service.StoreDeck(ctx.Request.Context(), userID, deckID, *req.StorageID, req.Boards)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrNoBoards, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrTargetStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"},
		)
		return
	}

	ctx.JSON(http.StatusOK, summary)
}
