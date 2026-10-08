package deck

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

type pendingCardResponse struct {
	ID              int     `json:"id"`
	DeckID          string  `json:"deck_id"`
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
	Added           string  `json:"added"`

	OwnedCopies       int `json:"owned_copies"`
	OwnedSamePrinting int `json:"owned_same_printing"`
}

func toPendingResponse(p PendingCard) pendingCardResponse {
	return pendingCardResponse{
		ID:              p.ID,
		DeckID:          p.DeckID,
		Name:            p.Name,
		ScryfallID:      p.ScryfallID,
		SetCode:         p.SetCode,
		CollectorNumber: p.CollectorNumber,
		Foil:            p.Foil,
		Quantity:        p.Quantity,
		ManaValue:       p.ManaValue,
		Colors:          p.Colors,
		CardType:        p.CardType,
		ColorIdentity:   p.ColorIdentity,
		Added:           p.Added.Format("2006-01-02 15:04:05"),

		OwnedCopies:       p.OwnedCopies,
		OwnedSamePrinting: p.OwnedSamePrinting,
	}
}

type addPendingCardRequest struct {
	Name            string  `json:"name" binding:"required"`
	ScryfallID      string  `json:"scryfall_id" binding:"required,uuid"`
	SetCode         string  `json:"set_code" binding:"required"`
	CollectorNumber string  `json:"collector_number" binding:"required"`
	Foil            bool    `json:"foil"`
	Quantity        *int    `json:"quantity" binding:"omitempty,gte=1,lte=100"`
	ManaValue       float64 `json:"mana_value" binding:"omitempty,gte=0"`
	Colors          *string `json:"colors"`
	CardType        *string `json:"card_type"`
	ColorIdentity   *string `json:"color_identity"`
}

var errInvalidPendingColors = errors.New("colors and color_identity must only contain the letters W, U, B, R and G")
var errInvalidPendingCardType = errors.New("card_type must be one of: Creature, Planeswalker, Battle, Instant, Sorcery, Artifact, Enchantment, Land, Other")

func normalizeLetters(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	upper := strings.ToUpper(*value)
	if strings.Trim(upper, "WUBRG") != "" {
		return nil, errInvalidPendingColors
	}
	normalized := scryfall.ColorCode(strings.Split(upper, ""))
	return &normalized, nil
}

func (r addPendingCardRequest) toDomain() (PendingCard, error) {
	colors, err := normalizeLetters(r.Colors)
	if err != nil {
		return PendingCard{}, err
	}
	identity, err := normalizeLetters(r.ColorIdentity)
	if err != nil {
		return PendingCard{}, err
	}
	if r.CardType != nil && !scryfall.IsPrimaryType(*r.CardType) {
		return PendingCard{}, errInvalidPendingCardType
	}
	quantity := 1
	if r.Quantity != nil {
		quantity = *r.Quantity
	}
	return PendingCard{
		Name:            r.Name,
		ScryfallID:      r.ScryfallID,
		SetCode:         r.SetCode,
		CollectorNumber: r.CollectorNumber,
		Foil:            r.Foil,
		Quantity:        quantity,
		ManaValue:       r.ManaValue,
		Colors:          colors,
		CardType:        r.CardType,
		ColorIdentity:   identity,
	}, nil
}

func (h *Handler) getPendingCards(ctx *gin.Context) {
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

	pending, err := h.service.GetPendingCards(ctx.Request.Context(), userID, deckID)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"})
		return
	}

	response := make([]pendingCardResponse, 0, len(pending))
	for _, p := range pending {
		response = append(response, toPendingResponse(p))
	}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) addPendingCard(ctx *gin.Context) {
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

	var req addPendingCardRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pending, err := req.toDomain()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.service.AddPendingCard(ctx.Request.Context(), userID, deckID, pending)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"})
		return
	}
	ctx.JSON(http.StatusCreated, toPendingResponse(created))
}

func (h *Handler) removePendingCard(ctx *gin.Context) {
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
	pendingID, err := strconv.Atoi(ctx.Param("pending_id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid pending_id"})
		return
	}

	if err := h.service.RemovePendingCard(ctx.Request.Context(), userID, deckID, pendingID); err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrPendingCardNotFound, Status: http.StatusNotFound, Message: "pending card not found"},
		)
		return
	}
	ctx.Status(http.StatusNoContent)
}

type updatePendingCardRequest struct {
	Quantity int `json:"quantity" binding:"required,gte=1,lte=1000"`
}

func (h *Handler) updatePendingCard(ctx *gin.Context) {
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
	pendingID, err := strconv.Atoi(ctx.Param("pending_id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid pending_id"})
		return
	}

	var req updatePendingCardRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.service.SetPendingQuantity(ctx.Request.Context(), userID, deckID, pendingID, req.Quantity)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrPendingCardNotFound, Status: http.StatusNotFound, Message: "pending card not found"},
		)
		return
	}
	ctx.JSON(http.StatusOK, toPendingResponse(updated))
}
