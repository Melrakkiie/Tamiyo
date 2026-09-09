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
	Updated   string `json:"updated"`
}

func toResponse(s Storage) storageResponse {
	return storageResponse{
		ID:        s.ID,
		Name:      s.Name,
		Type:      s.Type,
		CardCount: s.CardCount,
		Added:     s.Added.Format("2006-01-02 15:04:05"),
		Updated:   s.Updated.Format("2006-01-02 15:04:05"),
	}
}

type createStorageRequest struct {
	Name string `json:"name" binding:"required"`
	Type string `json:"type" binding:"required"`
}

func (r createStorageRequest) toDomain() Storage {
	return Storage{
		Name: r.Name,
		Type: r.Type,
	}
}

type storageService interface {
	GetAllStorages(ctx context.Context) ([]Storage, error)
	CreateStorage(ctx context.Context, storage Storage) (Storage, error)
}

type Handler struct {
	service storageService
}

func NewHandler(service storageService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/storage", h.getStorages)
	router.POST("/storage", h.createStorage)
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

func (h *Handler) createStorage(ctx *gin.Context) {
	var req createStorageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newStorage := req.toDomain()

	created, err := h.service.CreateStorage(ctx.Request.Context(), newStorage)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusCreated, toResponse(created))
}
