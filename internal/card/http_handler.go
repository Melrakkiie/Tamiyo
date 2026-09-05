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
		Added:           c.Added.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// cardService décrit ce dont le handler a besoin — permet de mocker facilement en test,
// sans dépendre de l'implémentation concrète du Service.
type cardService interface {
	GetAllCards(ctx context.Context) ([]Card, error)
}

// Handler expose les endpoints HTTP liés aux cartes.
type Handler struct {
	service cardService
}

func NewHandler(service cardService) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes attache les routes cartes au router Gin.
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/cards", h.getCards)
}

func (h *Handler) getCards(c *gin.Context) {
	cards, err := h.service.GetAllCards(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]cardResponse, 0, len(cards))
	for _, cd := range cards {
		response = append(response, toResponse(cd))
	}

	c.IndentedJSON(http.StatusOK, response)
}
