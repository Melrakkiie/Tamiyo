package deckshare

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/deck"
)

type likeStatusResponse struct {
	LikesCount int  `json:"likes_count"`
	LikedByMe  bool `json:"liked_by_me"`
}

type likedDeckResponse struct {
	publicDeckResponse
	LikedAt string `json:"liked_at"`
}

type likedDecksResponse struct {
	Data       []likedDeckResponse `json:"data"`
	Page       int                 `json:"page"`
	Limit      int                 `json:"limit"`
	Total      int                 `json:"total"`
	TotalPages int                 `json:"total_pages"`
}

func (h *Handler) respondLike(ctx *gin.Context, change func(context.Context, string, string) (deck.LikeStatus, error)) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	deckID, ok := parseDeckID(ctx)
	if !ok {
		return
	}
	status, err := change(ctx.Request.Context(), userID, deckID)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrLikeOwnDeck, Status: http.StatusBadRequest},
		)
		return
	}
	ctx.JSON(http.StatusOK, likeStatusResponse{LikesCount: status.Count, LikedByMe: status.LikedByMe})
}

func (h *Handler) likeStatus(ctx *gin.Context) {
	h.respondLike(ctx, h.service.LikeStatus)
}

func (h *Handler) likeDeck(ctx *gin.Context) {
	h.respondLike(ctx, h.service.LikeDeck)
}

func (h *Handler) unlikeDeck(ctx *gin.Context) {
	h.respondLike(ctx, h.service.UnlikeDeck)
}

func (h *Handler) likedDecks(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	page, err := positiveInt(ctx, "page", 1, 0)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := positiveInt(ctx, "limit", defaultBrowseLimit, maxBrowseLimit)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	decks, total, err := h.service.LikedDecks(ctx.Request.Context(), userID, page, limit)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}
	response := likedDecksResponse{
		Data:       make([]likedDeckResponse, 0, len(decks)),
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: (total + limit - 1) / limit,
	}
	for _, d := range decks {
		liked := likedDeckResponse{publicDeckResponse: toPublicDeckResponse(d)}
		if d.LikedAt != nil {
			liked.LikedAt = d.LikedAt.Format("2006-01-02 15:04:05")
		}
		response.Data = append(response.Data, liked)
	}
	ctx.JSON(http.StatusOK, response)
}
