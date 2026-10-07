package deck

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

const (
	defaultPage  = 1
	defaultLimit = 25
	maxLimit     = 100
)

type deckResponse struct {
	ID                   int     `json:"id"`
	Name                 string  `json:"name"`
	Format               string  `json:"format"`
	CommanderID          *int    `json:"commander_id"`
	CommanderPendingID   *int    `json:"commander_pending_id"`
	BackgroundScryfallID *string `json:"background_scryfall_id"`
	CommanderScryfallID  *string `json:"commander_scryfall_id"`
	CardCount            int     `json:"card_count"`
	PendingCount         int     `json:"pending_count"`
	Added                string  `json:"added"`
	Updated              string  `json:"updated"`
}

func toResponse(d Deck) deckResponse {
	return deckResponse{
		ID:                   d.ID,
		Name:                 d.Name,
		Format:               d.Format,
		CommanderID:          d.CommanderID,
		CommanderPendingID:   d.CommanderPendingID,
		BackgroundScryfallID: d.BackgroundScryfallID,
		CommanderScryfallID:  d.CommanderScryfallID,
		CardCount:            d.CardCount,
		PendingCount:         d.PendingCount,
		Added:                d.Added.Format("2006-01-02 15:04:05"),
		Updated:              d.Updated.Format("2006-01-02 15:04:05"),
	}
}

type paginatedDecksResponse struct {
	Data       []deckResponse `json:"data"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	Total      int            `json:"total"`
	TotalPages int            `json:"total_pages"`
}

type createDeckRequest struct {
	Name                 string  `json:"name" binding:"required"`
	Format               string  `json:"format" binding:"required"`
	CommanderID          *int    `json:"commander_id" binding:"omitempty,gt=0"`
	BackgroundScryfallID *string `json:"background_scryfall_id" binding:"omitempty,uuid"`
}

func (r createDeckRequest) toDomain() Deck {
	return Deck{
		Name:                 r.Name,
		Format:               r.Format,
		CommanderID:          r.CommanderID,
		BackgroundScryfallID: r.BackgroundScryfallID,
	}
}

type updateDeckRequest struct {
	Name                 *string `json:"name" binding:"omitempty"`
	Format               *string `json:"format" binding:"omitempty"`
	CommanderID          *int    `json:"commander_id" binding:"omitempty,gt=0"`
	CommanderPendingID   *int    `json:"commander_pending_id" binding:"omitempty,gt=0"`
	ClearCommanderID     bool    `json:"clear_commander_id"`
	BackgroundScryfallID *string `json:"background_scryfall_id" binding:"omitempty,uuid"`
	ClearBackground      bool    `json:"clear_background_scryfall_id"`
}

func (r updateDeckRequest) applyTo(d Deck) Deck {
	if r.Name != nil {
		d.Name = *r.Name
	}
	if r.Format != nil {
		d.Format = *r.Format
	}
	if r.ClearCommanderID {
		d.CommanderID = nil
		d.CommanderPendingID = nil
	} else if r.CommanderID != nil {
		d.CommanderID = r.CommanderID
		d.CommanderPendingID = nil
	} else if r.CommanderPendingID != nil {
		d.CommanderPendingID = r.CommanderPendingID
		d.CommanderID = nil
	}
	if r.ClearBackground {
		d.BackgroundScryfallID = nil
	} else if r.BackgroundScryfallID != nil {
		d.BackgroundScryfallID = r.BackgroundScryfallID
	}
	return d
}

type deckCardResponse struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	ScryfallID      string  `json:"scryfall_id"`
	SetCode         string  `json:"set_code"`
	CollectorNumber string  `json:"collector_number"`
	Foil            bool    `json:"foil"`
	StorageID       *int    `json:"storage_id"`
	ManaValue       float64 `json:"mana_value"`
	Colors          *string `json:"colors"`
	CardType        *string `json:"card_type"`
	ColorIdentity   *string `json:"color_identity"`
	Added           string  `json:"added"`
	Updated         string  `json:"updated"`
}

func toDeckCardResponse(dc DeckCard) deckCardResponse {
	return deckCardResponse{
		ID:              dc.ID,
		Name:            dc.Name,
		ScryfallID:      dc.ScryfallID,
		SetCode:         dc.SetCode,
		CollectorNumber: dc.CollectorNumber,
		Foil:            dc.Foil,
		StorageID:       dc.StorageID,
		ManaValue:       dc.ManaValue,
		Colors:          dc.Colors,
		CardType:        dc.CardType,
		ColorIdentity:   dc.ColorIdentity,
		Added:           dc.Added.Format("2006-01-02 15:04:05"),
		Updated:         dc.Updated.Format("2006-01-02 15:04:05"),
	}
}

type deckService interface {
	GetAllDecks(ctx context.Context, userID string, filter Filter) ([]Deck, int, error)
	GetDeck(ctx context.Context, userID string, id int) (Deck, error)
	CreateDeck(ctx context.Context, userID string, d Deck) (Deck, error)
	UpdateDeck(ctx context.Context, userID string, id int, req updateDeckRequest) (Deck, error)
	DeleteDeck(ctx context.Context, userID string, id int) error

	GetDeckCards(ctx context.Context, userID string, deckID int, sortField string, sortDesc bool) ([]DeckCard, error)
	PutCardInDeck(ctx context.Context, userID string, deckID int, cardID int) error
	RemoveCardFromDeck(ctx context.Context, userID string, deckID int, cardID int) error

	GetPendingCards(ctx context.Context, userID string, deckID int) ([]PendingCard, error)
	AddPendingCard(ctx context.Context, userID string, deckID int, p PendingCard) (PendingCard, error)
	RemovePendingCard(ctx context.Context, userID string, deckID, id int) error
}

type Handler struct {
	service deckService
}

func NewHandler(service deckService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/deck", h.getDecks)
	router.GET("/deck/:id", h.getDeck)
	router.POST("/deck", h.createDeck)
	router.PATCH("/deck/:id", h.updateDeck)
	router.DELETE("/deck/:id", h.deleteDeck)

	router.GET("/deck/:id/cards", h.getDeckCards)
	router.PUT("/deck/:id/cards/:card_id", h.putCardInDeck)
	router.DELETE("/deck/:id/cards/:card_id", h.removeCardFromDeck)

	router.GET("/deck/:id/pending", h.getPendingCards)
	router.POST("/deck/:id/pending", h.addPendingCard)
	router.DELETE("/deck/:id/pending/:pending_id", h.removePendingCard)
}

func (h *Handler) getDecks(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	page := defaultPage
	if raw := ctx.Query("page"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "page must be a positive integer"})
			return
		}
		page = parsed
	}

	limit := defaultLimit
	if raw := ctx.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxLimit {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("limit must be an integer between 1 and %d", maxLimit)})
			return
		}
		limit = parsed
	}

	sortField := "updated"
	sortDesc := true
	if raw := ctx.Query("sort"); raw != "" {
		field := raw
		desc := false
		if strings.HasPrefix(raw, "-") {
			desc = true
			field = raw[1:]
		}
		switch field {
		case "name", "added", "updated":
			sortField = field
			sortDesc = desc
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "sort must be one of: name, -name, added, -added, updated, -updated"})
			return
		}
	}

	filter := Filter{
		Format:    ctx.Query("format"),
		SortField: sortField,
		SortDesc:  sortDesc,
		Page:      page,
		Limit:     limit,
	}

	decks, total, err := h.service.GetAllDecks(ctx.Request.Context(), userID, filter)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}

	response := make([]deckResponse, 0, len(decks))
	for _, d := range decks {
		response = append(response, toResponse(d))
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	ctx.IndentedJSON(http.StatusOK, paginatedDecksResponse{
		Data:       response,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	})
}

func (h *Handler) getDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	deck, err := h.service.GetDeck(ctx.Request.Context(), userID, id)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"})
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(deck))
}

func (h *Handler) createDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var req createDeckRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.service.CreateDeck(ctx.Request.Context(), userID, req.toDomain())
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrCommanderNotFound, Status: http.StatusBadRequest, Message: "commander_id does not reference an existing card"})
		return
	}

	ctx.IndentedJSON(http.StatusCreated, toResponse(created))
}

func (h *Handler) updateDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req updateDeckRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.service.UpdateDeck(ctx.Request.Context(), userID, id, req)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrCommanderNotFound, Status: http.StatusBadRequest, Message: "commander_id does not reference an existing card"},
		)
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(updated))
}

func (h *Handler) deleteDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.DeleteDeck(ctx.Request.Context(), userID, id); err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"})
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (h *Handler) getDeckCards(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	sortField := "updated"
	sortDesc := true
	if raw := ctx.Query("sort"); raw != "" {
		field := raw
		desc := false
		if strings.HasPrefix(raw, "-") {
			desc = true
			field = raw[1:]
		}
		switch field {
		case "name", "added", "updated", "mana_value":
			sortField = field
			sortDesc = desc
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "sort must be one of: name, -name, added, -added, updated, -updated, mana_value, -mana_value"})
			return
		}
	}

	deckCards, err := h.service.GetDeckCards(ctx.Request.Context(), userID, id, sortField, sortDesc)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"})
		return
	}

	response := make([]deckCardResponse, 0, len(deckCards))
	for _, dc := range deckCards {
		response = append(response, toDeckCardResponse(dc))
	}
	ctx.IndentedJSON(http.StatusOK, response)
}

func (h *Handler) putCardInDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	deckID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid deck_id"})
		return
	}

	cardID, err := strconv.Atoi(ctx.Param("card_id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid card_id"})
		return
	}

	if err := h.service.PutCardInDeck(ctx.Request.Context(), userID, deckID, cardID); err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrCardNotFound, Status: http.StatusNotFound, Message: "card not found"},
		)
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (h *Handler) removeCardFromDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	deckID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid deck_id"})
		return
	}

	cardID, err := strconv.Atoi(ctx.Param("card_id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid card_id"})
		return
	}

	if err := h.service.RemoveCardFromDeck(ctx.Request.Context(), userID, deckID, cardID); err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"})
		return
	}

	ctx.Status(http.StatusNoContent)
}
