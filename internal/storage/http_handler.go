package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/auth"
)

const (
	defaultPage  = 1
	defaultLimit = 25
	maxLimit     = 100
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

type paginatedStorageResponse struct {
	Data       []storageResponse `json:"data"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	Total      int               `json:"total"`
	TotalPages int               `json:"total_pages"`
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
	GetAllStorages(ctx context.Context, userID string, filter Filter) ([]Storage, int, error)
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
		Type:      ctx.Query("type"),
		SortField: sortField,
		SortDesc:  sortDesc,
		Page:      page,
		Limit:     limit,
	}

	storages, total, err := h.service.GetAllStorages(ctx.Request.Context(), userID, filter)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]storageResponse, 0, len(storages))
	for _, s := range storages {
		response = append(response, toResponse(s))
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	ctx.IndentedJSON(http.StatusOK, paginatedStorageResponse{
		Data:       response,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	})
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
