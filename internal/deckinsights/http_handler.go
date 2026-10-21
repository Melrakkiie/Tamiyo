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
	router.GET("/deck/:id/legality", private(boxed(h.service.GetDeckLegality)))
	router.GET("/deck/:id/stats", private(boxed(h.service.GetDeckStats)))
	router.GET("/deck/:id/combos", private(boxed(h.service.GetDeckCombos)))
}

// RegisterSharedRoutes registers the insights of shared decks, readable by
// anyone, signed in or not.
func (h *Handler) RegisterSharedRoutes(router gin.IRoutes) {
	router.GET("/shared/decks/:id/legality", shared(boxed(h.service.GetDeckLegality)))
	router.GET("/shared/decks/:id/stats", shared(boxed(h.service.GetDeckStats)))
	router.GET("/shared/decks/:id/combos", shared(boxed(h.service.GetDeckCombos)))
}

type insight func(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (any, error)

func boxed[T any](compute func(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (T, error)) insight {
	return func(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (any, error) {
		return compute(ctx, viewerID, deckID, reach)
	}
}

// private serves insight to the signed-in user, for their own decks.
func private(compute insight) gin.HandlerFunc {
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

		respond(ctx, compute, userID, deckID, deck.ReachOwn, ctx.IndentedJSON)
	}
}

// shared serves insight to whoever is viewing, anonymous included, for shared
// decks. A malformed id answers like an unknown deck.
func shared(compute insight) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		viewerID, _ := auth.UserIDFromContext(ctx)
		deckID, valid := deck.ParseID(ctx.Param("id"))
		if !valid {
			respondError(ctx, deck.ErrNotFound)
			return
		}

		respond(ctx, compute, viewerID, deckID, deck.ReachShared, ctx.JSON)
	}
}

func respond(ctx *gin.Context, compute insight, viewerID string, deckID string, reach deck.Reach, render func(code int, body any)) {
	body, err := compute(ctx.Request.Context(), viewerID, deckID, reach)
	if err != nil {
		respondError(ctx, err)
		return
	}
	render(http.StatusOK, body)
}

func respondError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err,
		apierr.Mapping{Err: deck.ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
		apierr.Mapping{Err: ErrUnknownFormat, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrCombosUnavailable, Status: http.StatusBadGateway, Message: ErrCombosUnavailable.Error()},
	)
}
