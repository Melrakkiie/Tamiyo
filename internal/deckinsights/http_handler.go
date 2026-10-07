package deckinsights

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/deck"
)

type insightsService interface {
	GetDeckLegality(ctx context.Context, userID string, deckID string) (LegalityReport, error)
	GetDeckStats(ctx context.Context, userID string, deckID string) (DeckStats, error)
}

type Handler struct {
	service insightsService
}

func NewHandler(service insightsService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/deck/:id/legality", h.getDeckLegality)
	router.GET("/deck/:id/stats", h.getDeckStats)
}

func (h *Handler) getDeckLegality(ctx *gin.Context) {
	userID, deckID, ok := h.authAndParseID(ctx)
	if !ok {
		return
	}

	report, err := h.service.GetDeckLegality(ctx.Request.Context(), userID, deckID)
	if err != nil {
		h.respondError(ctx, err)
		return
	}

	ctx.IndentedJSON(http.StatusOK, report)
}

func (h *Handler) getDeckStats(ctx *gin.Context) {
	userID, deckID, ok := h.authAndParseID(ctx)
	if !ok {
		return
	}

	stats, err := h.service.GetDeckStats(ctx.Request.Context(), userID, deckID)
	if err != nil {
		h.respondError(ctx, err)
		return
	}

	ctx.IndentedJSON(http.StatusOK, stats)
}

func (h *Handler) authAndParseID(ctx *gin.Context) (userID string, deckID string, ok bool) {
	userID, authed := auth.UserIDFromContext(ctx)
	if !authed {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return "", "", false
	}

	deckID, valid := deck.ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", "", false
	}

	return userID, deckID, true
}

func (h *Handler) respondError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err,
		apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
		apierr.Mapping{Err: ErrUnknownFormat, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrScryfallUnavailable, Status: http.StatusBadGateway},
	)
}
