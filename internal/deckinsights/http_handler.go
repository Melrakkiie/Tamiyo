package deckinsights

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/auth"
)

type insightsService interface {
	GetDeckLegality(ctx context.Context, userID string, deckID int) (LegalityReport, error)
	GetDeckStats(ctx context.Context, userID string, deckID int) (DeckStats, error)
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

func (h *Handler) authAndParseID(ctx *gin.Context) (userID string, deckID int, ok bool) {
	userID, authed := auth.UserIDFromContext(ctx)
	if !authed {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return "", 0, false
	}

	deckID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", 0, false
	}

	return userID, deckID, true
}

func (h *Handler) respondError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDeckNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
	case errors.Is(err, ErrUnknownFormat):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrScryfallUnavailable):
		ctx.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	default:
		_ = ctx.Error(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
