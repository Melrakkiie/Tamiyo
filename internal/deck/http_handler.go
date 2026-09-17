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

type deckService interface {
	GetAllDecks(ctx context.Context) ([]Deck, error)
	GetDeck(ctx context.Context, id int) (Deck, error)
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
