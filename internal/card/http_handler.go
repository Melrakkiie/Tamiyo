package card

import (
	"context"
	"errors"
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

type updateCardRequest struct {
	Name            *string `json:"name" binding:"omitempty"`
	ScryfallID      *string `json:"scryfall_id" binding:"omitempty,uuid"`
	SetCode         *string `json:"set_code" binding:"omitempty"`
	CollectorNumber *int    `json:"collector_number" binding:"omitempty,gt=0"`
	Foil            *bool   `json:"foil" binding:"omitempty"`
	StorageID       *int    `json:"storage_id" binding:"omitempty,gt=0"`
}

func (r updateCardRequest) applyTo(c Card) Card {
	if r.Name != nil {
		c.Name = *r.Name
	}
	if r.ScryfallID != nil {
		c.ScryfallID = *r.ScryfallID
	}
	if r.SetCode != nil {
		c.SetCode = *r.SetCode
	}
	if r.CollectorNumber != nil {
		c.CollectorNumber = *r.CollectorNumber
	}
	if r.Foil != nil {
		c.Foil = *r.Foil
	}
	if r.StorageID != nil {
		c.StorageID = r.StorageID
	}
	return c
}

type cardService interface {
	GetAllCards(ctx context.Context, StorageID *int) ([]Card, error)
	GetCard(ctc context.Context, id int) (Card, error)
	CreateCard(ctx context.Context, c Card) (Card, error)
	UpdateCard(ctx context.Context, id int, req updateCardRequest) (Card, error)
	DeleteCard(ctx context.Context, id int) error
}

type Handler struct {
	service cardService
}

func NewHandler(service cardService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/cards", h.getCards)
	router.GET("/cards/:id", h.getCard)
	router.POST("/cards", h.createCard)
	router.PATCH("/cards/:id", h.updateCard)
	router.DELETE("/cards/:id", h.deleteCard)
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

func (h *Handler) getCard(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	card, err := h.service.GetCard(ctx.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(card))
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
		if errors.Is(err, ErrStorageNotFound) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id does not reference an existing storage"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusCreated, toResponse(created))
}

func (h *Handler) updateCard(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req updateCardRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.service.UpdateCard(ctx.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
			return
		}
		if errors.Is(err, ErrStorageNotFound) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id does not reference an existing storage"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(updated))
}

func (h *Handler) deleteCard(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.DeleteCard(ctx.Request.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}
