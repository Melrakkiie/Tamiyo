package storage

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/auth"
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
	return Storage{Name: r.Name, Type: r.Type}
}

type updateStorageRequest struct {
	Name *string `json:"name" binding:"omitempty"`
	Type *string `json:"type" binding:"omitempty"`
}

func (r updateStorageRequest) applyTo(s Storage) Storage {
	if r.Name != nil {
		s.Name = *r.Name
	}
	if r.Type != nil {
		s.Type = *r.Type
	}
	return s
}

type storageService interface {
	GetAllStorages(ctx context.Context, userID string, filter Filter) ([]Storage, error)
	GetStorage(ctx context.Context, userID string, id int) (Storage, error)
	CreateStorage(ctx context.Context, userID string, storage Storage) (Storage, error)
	UpdateStorage(ctx context.Context, userID string, id int, req updateStorageRequest) (Storage, error)
	DeleteStorage(ctx context.Context, userID string, id int) error
}

type Handler struct {
	service storageService
}

func NewHandler(service storageService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/storage", h.getStorages)
	router.GET("/storage/:id", h.getStorage)
	router.POST("/storage", h.createStorage)
	router.PATCH("/storage/:id", h.updateStorage)
	router.DELETE("/storage/:id", h.deleteStorage)
}

func (h *Handler) getStorages(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	filter := Filter{Type: ctx.Query("type")}

	storages, err := h.service.GetAllStorages(ctx.Request.Context(), userID, filter)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]storageResponse, 0, len(storages))
	for _, s := range storages {
		response = append(response, toResponse(s))
	}
	ctx.IndentedJSON(http.StatusOK, response)
}

func (h *Handler) getStorage(ctx *gin.Context) {
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

	s, err := h.service.GetStorage(ctx.Request.Context(), userID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "storage not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(s))
}

func (h *Handler) createStorage(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var req createStorageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.service.CreateStorage(ctx.Request.Context(), userID, req.toDomain())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusCreated, toResponse(created))
}

func (h *Handler) updateStorage(ctx *gin.Context) {
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

	var req updateStorageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.service.UpdateStorage(ctx.Request.Context(), userID, id, req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "storage not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(updated))
}

func (h *Handler) deleteStorage(ctx *gin.Context) {
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

	if err := h.service.DeleteStorage(ctx.Request.Context(), userID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "storage not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}
