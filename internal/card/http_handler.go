package card

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type cardResponse struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	ScryfallID      string `json:"scryfall_id"`
	SetCode         string `json:"set_code"`
	CollectorNumber int    `json:"collector_number"`
	Foil            bool   `json:"foil"`
	BinderName      string `json:"binder_name"`
	BinderType      string `json:"binder_type"`
	Added           string `json:"added"`
}

func toResponse(c Card) cardResponse {
	return cardResponse{
		ID:              c.ID,
		Name:            c.Name,
		ScryfallID:      c.ScryfallID,
		SetCode:         c.SetCode,
		CollectorNumber: c.CollectorNumber,
		Foil:            c.Foil,
		BinderName:      c.BinderName,
		BinderType:      c.BinderType,
		Added:           c.Added.Format("2006-01-02 15:04:05"),
	}
}

type createCardRequest struct {
	Name            string `json:"name" binding:"required"`
	ScryfallID      string `json:"scryfall_id" binding:"required,uuid"`
	SetCode         string `json:"set_code" binding:"required"`
	CollectorNumber int    `json:"collector_number" binding:"required,gt=0"`
	Foil            bool   `json:"foil"`
	BinderName      string `json:"binder_name" binding:"required"`
	BinderType      string `json:"binder_type" binding:"required"`
}

func (r createCardRequest) toDomain() Card {
	return Card{
		Name:            r.Name,
		ScryfallID:      r.ScryfallID,
		SetCode:         r.SetCode,
		CollectorNumber: r.CollectorNumber,
		Foil:            r.Foil,
		BinderName:      r.BinderName,
		BinderType:      r.BinderType,
	}
}

type cardService interface {
	GetAllCards(ctx context.Context) ([]Card, error)
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
	cards, err := h.service.GetAllCards(ctx.Request.Context())
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

func (h *Handler) createCard(c *gin.Context) {
	var req createCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newCard := req.toDomain()

	created, err := h.service.CreateCard(c.Request.Context(), newCard)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.IndentedJSON(http.StatusCreated, toResponse(created))
}
