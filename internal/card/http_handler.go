package card

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type cardResponse struct {
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

func toResponse(c Card) cardResponse {
	return cardResponse{
		ID:              c.ID,
		Name:            c.Name,
		ScryfallID:      c.ScryfallID,
		SetCode:         c.SetCode,
		CollectorNumber: c.CollectorNumber,
		Foil:            c.Foil,
		StorageID:       c.StorageID,
		Added:           c.Added.Format("2006-01-02 15:04:05"),
		Updated:         c.Updated.Format("2006-01-02 15:04:05"),
	}
}

type createCardRequest struct {
	Name            string `json:"name" binding:"required"`
	ScryfallID      string `json:"scryfall_id" binding:"required,uuid"`
	SetCode         string `json:"set_code" binding:"required"`
	CollectorNumber int    `json:"collector_number" binding:"required,gt=0"`
	Foil            bool   `json:"foil"`
	StorageID       *int   `json:"storage_id" binding:"omitempty,gt=0"`
}

func (r createCardRequest) toDomain() Card {
	return Card{
		Name:            r.Name,
		ScryfallID:      r.ScryfallID,
		SetCode:         r.SetCode,
		CollectorNumber: r.CollectorNumber,
		Foil:            r.Foil,
		StorageID:       r.StorageID,
	}
}

type cardService interface {
	GetAllCards(ctx context.Context, StorageID *int) ([]Card, error)
	CreateCard(ctx context.Context, c Card) (Card, error)
}

type Handler struct {
	service cardService
}

func NewHandler(service cardService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/cards", h.getCards)
	router.POST("/cards", h.createCard)
}

func (h *Handler) getCards(ctx *gin.Context) {
	var storageID *int

	if raw := ctx.Query("storage_id"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id must be a valid integer"})
			return
		}
		storageID = &parsed
	}

	cards, err := h.service.GetAllCards(ctx.Request.Context(), storageID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]cardResponse, 0, len(cards))
	for _, cd := range cards {
		response = append(response, toResponse(cd))
	}
	ctx.IndentedJSON(http.StatusOK, response)
}

func (h *Handler) createCard(ctx *gin.Context) {
	var req createCardRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newCard := req.toDomain()

	created, err := h.service.CreateCard(ctx.Request.Context(), newCard)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusCreated, toResponse(created))
}
