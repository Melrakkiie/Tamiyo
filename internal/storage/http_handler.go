package storage

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type storageResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	CardCount int    `json:"card_count"`
	Added     string `json:"added"`
}

func toResponse(s Storage) storageResponse {
	return storageResponse{
		ID:        s.ID,
		Name:      s.Name,
		Type:      s.Type,
		CardCount: s.CardCount,
		Added:     s.Added.Format("2006-01-02 15:04:05"),
	}
}

type storageService interface {
	GetAllStorages(ctx context.Context) ([]Storage, error)
}

type Handler struct {
	service storageService
}

func NewHandler(service storageService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/storage", h.getStorages)
}

func (h *Handler) getStorages(ctx *gin.Context) {
	storages, err := h.service.GetAllStorages(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]storageResponse, 0, len(storages))
	for _, cd := range storages {
		response = append(response, toResponse(cd))
	}
	ctx.IndentedJSON(http.StatusOK, response)
}
