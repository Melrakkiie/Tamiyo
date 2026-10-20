package deck

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

type switchService interface {
	SwitchFinish(ctx context.Context, userID string, deckID string, cards CardSelection, foil bool) (ReplacementSummary, error)
	SwitchProxy(ctx context.Context, userID string, deckID string, cards CardSelection, proxy bool) (ReplacementSummary, error)
	SplitPrintings(ctx context.Context, userID string, deckID string, cards CardSelection, printings []PrintingChoice) (ReplacementSummary, error)
}

type switchFinishRequest struct {
	CardIDs    []int `json:"card_ids"`
	PendingIDs []int `json:"pending_ids"`
	Foil       *bool `json:"foil"`
}

type switchProxyRequest struct {
	CardIDs    []int `json:"card_ids"`
	PendingIDs []int `json:"pending_ids"`
	Proxy      *bool `json:"proxy"`
}

type splitPrintingsRequest struct {
	CardIDs    []int                   `json:"card_ids"`
	PendingIDs []int                   `json:"pending_ids"`
	Printings  []printingChoiceRequest `json:"printings"`
}

type printingChoiceRequest struct {
	ScryfallID      string `json:"scryfall_id"`
	SetCode         string `json:"set_code"`
	CollectorNumber string `json:"collector_number"`
	Foil            bool   `json:"foil"`
}

// switchResponse keeps the import summary's shape these routes always
// answered with.
type switchResponse struct {
	CardsCreated    int `json:"cards_created"`
	CardsLinked     int `json:"cards_linked"`
	CardsPending    int `json:"cards_pending"`
	CardsSkipped    int `json:"cards_skipped"`
	StoragesCreated int `json:"storages_created"`
	DecksCreated    int `json:"decks_created"`
	CardsRemoved    int `json:"cards_removed,omitempty"`
}

func (h *Handler) registerSwitchRoutes(router gin.IRoutes) {
	router.POST("/deck/:id/switch-foil", h.switchFinish)
	router.POST("/deck/:id/switch-proxy", h.switchProxy)
	router.POST("/deck/:id/split-printings", h.splitPrintings)
}

func (h *Handler) switchFinish(ctx *gin.Context) {
	var req switchFinishRequest
	h.switchDeckCards(ctx, &req, "foil and at least one of card_ids or pending_ids are required",
		func() bool { return req.Foil != nil && len(req.CardIDs)+len(req.PendingIDs) > 0 },
		func(c context.Context, userID, deckID string) (ReplacementSummary, error) {
			return h.service.SwitchFinish(c, userID, deckID, CardSelection{CardIDs: req.CardIDs, PendingIDs: req.PendingIDs}, *req.Foil)
		})
}

func (h *Handler) switchProxy(ctx *gin.Context) {
	var req switchProxyRequest
	h.switchDeckCards(ctx, &req, "proxy and at least one of card_ids or pending_ids are required",
		func() bool { return req.Proxy != nil && len(req.CardIDs)+len(req.PendingIDs) > 0 },
		func(c context.Context, userID, deckID string) (ReplacementSummary, error) {
			return h.service.SwitchProxy(c, userID, deckID, CardSelection{CardIDs: req.CardIDs, PendingIDs: req.PendingIDs}, *req.Proxy)
		})
}

func (h *Handler) splitPrintings(ctx *gin.Context) {
	var req splitPrintingsRequest
	h.switchDeckCards(ctx, &req, "printings, each with a scryfall_id, and at least one of card_ids or pending_ids are required",
		func() bool { return len(req.CardIDs)+len(req.PendingIDs) > 0 && validPrintings(req.Printings) },
		func(c context.Context, userID, deckID string) (ReplacementSummary, error) {
			printings := make([]PrintingChoice, 0, len(req.Printings))
			for _, p := range req.Printings {
				printings = append(printings, PrintingChoice(p))
			}
			return h.service.SplitPrintings(c, userID, deckID, CardSelection{CardIDs: req.CardIDs, PendingIDs: req.PendingIDs}, printings)
		})
}

func (h *Handler) switchDeckCards(ctx *gin.Context, req any, invalid string, complete func() bool, run func(c context.Context, userID, deckID string) (ReplacementSummary, error)) {
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
	if err := json.NewDecoder(ctx.Request.Body).Decode(req); err != nil || !complete() {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": invalid})
		return
	}

	summary, err := run(ctx.Request.Context(), userID, deckID)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrCardsNotInDeck, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrSplitMismatch, Status: http.StatusBadRequest},
		)
		return
	}
	ctx.JSON(http.StatusOK, switchResponse{CardsLinked: summary.CardsLinked, CardsPending: summary.CardsPending, CardsRemoved: summary.CardsRemoved})
}

func validPrintings(printings []printingChoiceRequest) bool {
	if len(printings) == 0 {
		return false
	}
	for _, p := range printings {
		if strings.TrimSpace(p.ScryfallID) == "" {
			return false
		}
	}
	return true
}
