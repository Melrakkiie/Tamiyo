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
	Colors         *string `json:"colors"`
	CardType       *string `json:"card_type"`
	ColorIdentity  *string `json:"color_identity"`
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
	router.GET("/deck/:id/content", h.visibleDeck)
	router.GET("/deck/:id/compare/:other_id", h.compareDecks)
	router.GET("/deck/:id/ownership", h.collectionOwnership)
	router.GET("/deck/:id/like", h.likeStatus)
	router.PUT("/deck/:id/like", h.likeDeck)
	router.DELETE("/deck/:id/like", h.unlikeDeck)
	router.GET("/auth/me/liked-decks", h.likedDecks)
}

func (h *Handler) visibleDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	deckID, ok := parseDeckID(ctx)
	if !ok {
		return
	}

	visible, err := h.service.GetVisibleDeck(ctx.Request.Context(), userID, deckID)
	if err != nil {
		h.respondError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, toResponse(visible))
}

type ownedCardResponse struct {
	Name  string `json:"name"`
	Owned int    `json:"owned"`
}

type ownershipResponse struct {
	Cards []ownedCardResponse `json:"cards"`
}

func (h *Handler) collectionOwnership(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	deckID, ok := parseDeckID(ctx)
	if !ok {
		return
	}

	owned, err := h.service.CollectionOwnership(ctx.Request.Context(), userID, deckID)
	if err != nil {
		h.respondError(ctx, err)
		return
	}

	response := ownershipResponse{Cards: make([]ownedCardResponse, 0, len(owned))}
	for _, c := range owned {
		response.Cards = append(response.Cards, ownedCardResponse(c))
	}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) compareDecks(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	h.respondComparison(ctx, userID, http.StatusBadRequest)
}

func (h *Handler) compareSharedDecks(ctx *gin.Context) {
	h.respondComparison(ctx, "", http.StatusNotFound)
}

func (h *Handler) respondComparison(ctx *gin.Context, userID string, invalidIDStatus int) {
	deckID, validDeck := deck.ParseID(ctx.Param("id"))
	otherID, validOther := deck.ParseID(ctx.Param("other_id"))
	if !validDeck || !validOther {
		message := "invalid id"
		if invalidIDStatus == http.StatusNotFound {
			message = "deck not found"
		}
		ctx.JSON(invalidIDStatus, gin.H{"error": message})
		return
	}

	comparison, err := h.service.CompareDecks(ctx.Request.Context(), userID, deckID, otherID)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: deck.ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"})
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
