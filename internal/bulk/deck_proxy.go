package bulk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/deck"
)

type SwitchProxyRequest struct {
	CardIDs    []int `json:"card_ids"`
	PendingIDs []int `json:"pending_ids"`
	Proxy      *bool `json:"proxy"`
}

type ProxySwitch struct {
	CardIDs    []int
	PendingIDs []int
	Proxy      bool
}

func (s *Service) SwitchProxy(ctx context.Context, userID string, deckID string, req ProxySwitch) (Summary, error) {
	return s.switchCards(ctx, userID, deckID, req.CardIDs, req.PendingIDs, func(p deck.PendingCard) deck.PendingCard {
		p.Proxy = req.Proxy
		return p
	})
}

func (h *Handler) switchProxy(ctx *gin.Context) {
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
	var req SwitchProxyRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil || req.Proxy == nil || len(req.CardIDs)+len(req.PendingIDs) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy and at least one of card_ids or pending_ids are required"})
		return
	}

	summary, err := h.service.SwitchProxy(ctx.Request.Context(), userID, deckID, ProxySwitch{CardIDs: req.CardIDs, PendingIDs: req.PendingIDs, Proxy: *req.Proxy})
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrCardsNotInDeck, Status: http.StatusBadRequest},
		)
		return
	}
	ctx.JSON(http.StatusOK, summary)
}
