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
	GetDeckLegality(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (LegalityReport, error)
	GetDeckStats(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (DeckStats, error)
	GetDeckCombos(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (DeckCombos, error)
}

type Handler struct {
	service insightsService
}

func NewHandler(service insightsService) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers the insights of the signed-in user's own decks;
// router must require authentication.
func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/deck/:id/legality", h.private(h.legality))
	router.GET("/deck/:id/stats", h.private(h.stats))
	router.GET("/deck/:id/combos", h.private(h.combos))
}

// RegisterSharedRoutes registers the insights of shared decks, readable by
// anyone, signed in or not.
func (h *Handler) RegisterSharedRoutes(router gin.IRoutes) {
	router.GET("/shared/decks/:id/legality", h.shared(h.legality))
	router.GET("/shared/decks/:id/stats", h.shared(h.stats))
	router.GET("/shared/decks/:id/combos", h.shared(h.combos))
}

// insight computes one insight of a deck, read by viewerID within reach.
type insight func(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (any, error)

func (h *Handler) legality(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (any, error) {
	return h.service.GetDeckLegality(ctx, viewerID, deckID, reach)
}

func (h *Handler) stats(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (any, error) {
	return h.service.GetDeckStats(ctx, viewerID, deckID, reach)
}

func (h *Handler) combos(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (any, error) {
	return h.service.GetDeckCombos(ctx, viewerID, deckID, reach)
}

// private serves insight to the signed-in user, for their own decks.
func (h *Handler) private(compute insight) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userID, authed := auth.UserIDFromContext(ctx)
		if !authed {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}
		deckID, valid := deck.ParseID(ctx.Param("id"))
		if !valid {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		body, err := compute(ctx.Request.Context(), userID, deckID, deck.ReachOwn)
		if err != nil {
			respondError(ctx, err)
			return
		}
		ctx.IndentedJSON(http.StatusOK, body)
	}
}

// shared serves insight to whoever is viewing, anonymous included, for shared
// decks. A malformed id answers like an unknown deck.
func (h *Handler) shared(compute insight) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		viewerID, _ := auth.UserIDFromContext(ctx)
		deckID, valid := deck.ParseID(ctx.Param("id"))
		if !valid {
			respondError(ctx, deck.ErrNotFound)
			return
		}

		body, err := compute(ctx.Request.Context(), viewerID, deckID, deck.ReachShared)
		if err != nil {
			respondError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, body)
	}
}

func respondError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err,
		apierr.Mapping{Err: deck.ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
		apierr.Mapping{Err: ErrUnknownFormat, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrCombosUnavailable, Status: http.StatusBadGateway, Message: ErrCombosUnavailable.Error()},
	)
}
