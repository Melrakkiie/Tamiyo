package deck

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type deckResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Format      string `json:"format"`
	CommanderID *int   `json:"commander_id"`
	CardCount   int    `json:"card_count"`
	Added       string `json:"added"`
	Updated     string `json:"updated"`
}

func toResponse(d Deck) deckResponse {
	return deckResponse{
		ID:          d.ID,
		Name:        d.Name,
		Format:      d.Format,
		CommanderID: d.CommanderID,
		CardCount:   d.CardCount,
		Added:       d.Added.Format("2006-01-02 15:04:05"),
		Updated:     d.Updated.Format("2006-01-02 15:04:05"),
	}
}

type createDeckRequest struct {
	Name        string `json:"name" binding:"required"`
	Format      string `json:"format" binding:"required"`
	CommanderID *int   `json:"commander_id" binding:"omitempty,gt=0"`
}

func (r createDeckRequest) toDomain() Deck {
	return Deck{
		Name:        r.Name,
		Format:      r.Format,
		CommanderID: r.CommanderID,
	}
}

type updateDeckRequest struct {
	Name             *string `json:"name" binding:"omitempty"`
	Format           *string `json:"format" binding:"omitempty"`
	CommanderID      *int    `json:"commander_id" binding:"omitempty,gt=0"`
	ClearCommanderID bool    `json:"clear_storage_id"`
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
	} else if r.CommanderID != nil {
		d.CommanderID = r.CommanderID
	}
	return d
}

type deckCardResponse struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	ScryfallID      string `json:"scryfall_id"`
	SetCode         string `json:"set_code"`
	CollectorNumber int    `json:"collector_number"`
	Foil            bool   `json:"foil"`
	StorageID       *int   `json:"storage_id"`
	Added           string `json:"added"`
	Updated         string `json:"updated"`
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
		Added:           dc.Added.Format("2006-01-02 15:04:05"),
		Updated:         dc.Updated.Format("2006-01-02 15:04:05"),
	}
}

type deckService interface {
	GetAllDecks(ctx context.Context) ([]Deck, error)
	GetDeck(ctx context.Context, id int) (Deck, error)
	CreateDeck(ctx context.Context, d Deck) (Deck, error)
	UpdateDeck(ctx context.Context, id int, req updateDeckRequest) (Deck, error)
	DeleteDeck(ctx context.Context, id int) error

	GetDeckCards(ctx context.Context, deckID int) ([]DeckCard, error)
}

type Handler struct {
	service deckService
}

func NewHandler(service deckService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/deck", h.getDecks)
	router.GET("/deck/:id", h.getDeck)
	router.POST("/deck", h.createDeck)
	router.PATCH("/deck/:id", h.updateDeck)
	router.DELETE("/deck/:id", h.deleteDeck)

	router.GET("/deck/:id/cards", h.getDeckCards)
}

func (h *Handler) getDecks(ctx *gin.Context) {
	decks, err := h.service.GetAllDecks(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]deckResponse, 0, len(decks))
	for _, d := range decks {
		response = append(response, toResponse(d))
	}
	ctx.IndentedJSON(http.StatusOK, response)
}

func (h *Handler) getDeck(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	deck, err := h.service.GetDeck(ctx.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusCreated, toResponse(deck))
}

func (h *Handler) createDeck(ctx *gin.Context) {
	var req createDeckRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newDeck := req.toDomain()

	created, err := h.service.CreateDeck(ctx.Request.Context(), newDeck)
	if err != nil {
		if errors.Is(err, ErrCommanderNotFound) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "commander_id does not reference an existing card"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusCreated, toResponse(created))
}

func (h *Handler) updateDeck(ctx *gin.Context) {
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

	updated, err := h.service.UpdateDeck(ctx.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
			return
		}
		if errors.Is(err, ErrCommanderNotFound) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "commander_id does not reference an existing card"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(updated))
}

func (h *Handler) deleteDeck(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.DeleteDeck(ctx.Request.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (h *Handler) getDeckCards(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	deckCards, err := h.service.GetDeckCards(ctx.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]deckCardResponse, 0, len(deckCards))
	for _, dc := range deckCards {
		response = append(response, toDeckCardResponse(dc))
	}
	ctx.IndentedJSON(http.StatusOK, response)
}
