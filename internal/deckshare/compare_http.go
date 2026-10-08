package deckshare

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/deck"
)

type comparedDeckResponse struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Format     string        `json:"format"`
	Visibility string        `json:"visibility"`
	Owner      ownerResponse `json:"owner"`
	Mine       bool          `json:"mine"`
	CardCount  int           `json:"card_count"`
}

type comparedCardResponse struct {
	Name           string  `json:"name"`
	ScryfallID     string  `json:"scryfall_id"`
	ManaValue      float64 `json:"mana_value"`
	CardType       *string `json:"card_type"`
	Quantity       int     `json:"quantity"`
	OtherQuantity  int     `json:"other_quantity"`
	Commander      bool    `json:"commander"`
	OtherCommander bool    `json:"other_commander"`
}

type comparisonResponse struct {
	Deck        comparedDeckResponse   `json:"deck"`
	Other       comparedDeckResponse   `json:"other"`
	Common      []comparedCardResponse `json:"common"`
	OnlyInDeck  []comparedCardResponse `json:"only_in_deck"`
	OnlyInOther []comparedCardResponse `json:"only_in_other"`
}

func toComparedDeckResponse(c ComparedDeck) comparedDeckResponse {
	return comparedDeckResponse{
		ID:         c.Deck.ID,
		Name:       c.Deck.Name,
		Format:     c.Deck.Format,
		Visibility: c.Deck.Visibility,
		Owner:      ownerResponse(c.Owner),
		Mine:       c.Mine,
		CardCount:  c.CardCount,
	}
}

func toComparedCardsResponse(cards []ComparedCard) []comparedCardResponse {
	out := make([]comparedCardResponse, 0, len(cards))
	for _, c := range cards {
		out = append(out, comparedCardResponse(c))
	}
	return out
}

func (h *Handler) RegisterProtectedRoutes(router gin.IRoutes) {
	router.GET("/deck/:id/compare/:other_id", h.compareDecks)
}

func (h *Handler) compareDecks(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	deckID, validDeck := deck.ParseID(ctx.Param("id"))
	otherID, validOther := deck.ParseID(ctx.Param("other_id"))
	if !validDeck || !validOther {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	comparison, err := h.service.CompareDecks(ctx.Request.Context(), userID, deckID, otherID)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"})
		return
	}

	ctx.JSON(http.StatusOK, comparisonResponse{
		Deck:        toComparedDeckResponse(comparison.Deck),
		Other:       toComparedDeckResponse(comparison.Other),
		Common:      toComparedCardsResponse(comparison.Common),
		OnlyInDeck:  toComparedCardsResponse(comparison.OnlyInDeck),
		OnlyInOther: toComparedCardsResponse(comparison.OnlyInOther),
	})
}
