package bulk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/storage"
)

type TransferRequest struct {
	StorageID *int `json:"storage_id"`
}

func (s *Service) TransferStorage(ctx context.Context, userID string, fromStorageID, toStorageID int) (StoreSummary, error) {
	if fromStorageID == toStorageID {
		return StoreSummary{}, ErrSameStorage
	}
	if _, err := s.storages.GetStorage(ctx, userID, fromStorageID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return StoreSummary{}, ErrStorageNotFound
		}
		return StoreSummary{}, err
	}
	if err := s.checkStorage(ctx, userID, toStorageID); err != nil {
		return StoreSummary{}, err
	}

	moved, err := s.cards.MoveStorageCards(ctx, userID, fromStorageID, toStorageID)
	if err != nil {
		if errors.Is(err, card.ErrStorageNotFound) {
			return StoreSummary{}, ErrTargetStorageNotFound
		}
		return StoreSummary{}, err
	}
	return StoreSummary{CardsMoved: moved}, nil
}

func (h *Handler) transferStorage(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	storageID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req TransferRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.StorageID == nil || *req.StorageID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id is required and must be a positive integer"})
		return
	}

	summary, err := h.service.TransferStorage(ctx.Request.Context(), userID, storageID, *req.StorageID)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrStorageNotFound, Status: http.StatusNotFound, Message: "storage not found"},
			apierr.Mapping{Err: ErrSameStorage, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrTargetStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"},
		)
		return
	}

	ctx.JSON(http.StatusOK, summary)
}
