package deckshare

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/deckinsights"
)

type sharedDeckService interface {
	GetSharedDeck(ctx context.Context, deckID string) (SharedDeck, error)
	GetSharedDeckLegality(ctx context.Context, deckID string) (deckinsights.LegalityReport, error)
	GetSharedDeckStats(ctx context.Context, deckID string) (deckinsights.DeckStats, error)
	CompareDecks(ctx context.Context, userID string, deckID string, otherID string) (Comparison, error)
}

type deckResponse struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Format               string  `json:"format"`
	Visibility           string  `json:"visibility"`
	BackgroundScryfallID *string `json:"background_scryfall_id"`
	CommanderScryfallID  *string `json:"commander_scryfall_id"`
	CardCount            int     `json:"card_count"`
	Added                string  `json:"added"`
	Updated              string  `json:"updated"`
}

type ownerResponse struct {
	ID               string  `json:"id"`
	DisplayName      *string `json:"display_name"`
	AvatarScryfallID *string `json:"avatar_scryfall_id"`
}

type cardResponse struct {
	Name            string  `json:"name"`
	ScryfallID      string  `json:"scryfall_id"`
	SetCode         string  `json:"set_code"`
	CollectorNumber string  `json:"collector_number"`
	Foil            bool    `json:"foil"`
	Quantity        int     `json:"quantity"`
	ManaValue       float64 `json:"mana_value"`
	Colors          *string `json:"colors"`
	CardType        *string `json:"card_type"`
	ColorIdentity   *string `json:"color_identity"`
	Commander       bool    `json:"commander"`
}

type sharedDeckResponse struct {
	Deck  deckResponse   `json:"deck"`
	Owner ownerResponse  `json:"owner"`
	Cards []cardResponse `json:"cards"`
}

func toResponse(shared SharedDeck) sharedDeckResponse {
	cards := make([]cardResponse, 0, len(shared.Cards))
	total := 0
	for _, c := range shared.Cards {
		total += c.Quantity
		cards = append(cards, cardResponse(c))
	}
	d := shared.Deck
	return sharedDeckResponse{
		Deck: deckResponse{
			ID:                   d.ID,
			Name:                 d.Name,
			Format:               d.Format,
			Visibility:           d.Visibility,
			BackgroundScryfallID: d.BackgroundScryfallID,
			CommanderScryfallID:  d.CommanderScryfallID,
			CardCount:            total,
			Added:                d.Added.Format("2006-01-02 15:04:05"),
			Updated:              d.Updated.Format("2006-01-02 15:04:05"),
		},
		Owner: ownerResponse(shared.Owner),
		Cards: cards,
	}
}

type Handler struct {
	service sharedDeckService
}

func NewHandler(service sharedDeckService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/shared/decks/:id", h.getSharedDeck)
	router.GET("/shared/decks/:id/legality", h.getSharedDeckLegality)
	router.GET("/shared/decks/:id/stats", h.getSharedDeckStats)
}

func parseDeckID(ctx *gin.Context) (string, bool) {
	deckID, valid := deck.ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
		return "", false
	}
	return deckID, true
}

func (h *Handler) getSharedDeck(ctx *gin.Context) {
	deckID, ok := parseDeckID(ctx)
	if !ok {
		return
	}

	shared, err := h.service.GetSharedDeck(ctx.Request.Context(), deckID)
	if err != nil {
		h.respondError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, toResponse(shared))
}

func (h *Handler) getSharedDeckLegality(ctx *gin.Context) {
	deckID, ok := parseDeckID(ctx)
	if !ok {
		return
	}

	report, err := h.service.GetSharedDeckLegality(ctx.Request.Context(), deckID)
	if err != nil {
		h.respondError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, report)
}

func (h *Handler) getSharedDeckStats(ctx *gin.Context) {
	deckID, ok := parseDeckID(ctx)
	if !ok {
		return
	}

	stats, err := h.service.GetSharedDeckStats(ctx.Request.Context(), deckID)
	if err != nil {
		h.respondError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, stats)
}

func (h *Handler) respondError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err,
		apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
		apierr.Mapping{Err: deckinsights.ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
		apierr.Mapping{Err: deckinsights.ErrUnknownFormat, Status: http.StatusBadRequest},
		apierr.Mapping{Err: deckinsights.ErrScryfallUnavailable, Status: http.StatusBadGateway},
	)
}
