package bulk

import (
	"context"

	"github.com/gin-gonic/gin"

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
	var req SwitchProxyRequest
	h.switchDeckCards(ctx, "proxy", &req,
		func() bool { return req.Proxy != nil && len(req.CardIDs)+len(req.PendingIDs) > 0 },
		func(c context.Context, userID, deckID string) (Summary, error) {
			return h.service.SwitchProxy(c, userID, deckID, ProxySwitch{CardIDs: req.CardIDs, PendingIDs: req.PendingIDs, Proxy: *req.Proxy})
		})
}
