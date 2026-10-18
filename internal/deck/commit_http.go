package deck

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

type commitPendingRequest struct {
	StorageID *int `json:"storage_id"`
	PendingID *int `json:"pending_id"`
	Quantity  *int `json:"quantity"`
}

type commitPendingResponse struct {
	CardsCreated int `json:"cards_created"`
}

func (h *Handler) commitPendingCards(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	deckID, valid := ParseID(ctx.Param("id"))
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

	summary, err := h.service.CommitPendingCards(ctx.Request.Context(), userID, deckID, PendingCommit(req))
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrPendingCardNotFound, Status: http.StatusNotFound, Message: "pending card not found"},
			apierr.Mapping{Err: ErrTargetStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"},
		)
		return
	}

	ctx.JSON(http.StatusOK, commitPendingResponse(summary))
}
