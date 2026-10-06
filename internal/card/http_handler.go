package card

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

const (
	defaultPage  = 1
	defaultLimit = 25
	maxLimit     = 100
)

type cardResponse struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	ScryfallID      string  `json:"scryfall_id"`
	SetCode         string  `json:"set_code"`
	CollectorNumber string  `json:"collector_number"`
	Foil            bool    `json:"foil"`
	StorageID       *int    `json:"storage_id"`
	ManaValue       float64 `json:"mana_value"`
	Added           string  `json:"added"`
	Updated         string  `json:"updated"`
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
		ManaValue:       c.ManaValue,
		Added:           c.Added.Format("2006-01-02 15:04:05"),
		Updated:         c.Updated.Format("2006-01-02 15:04:05"),
	}
}

type paginatedCardsResponse struct {
	Data       []cardResponse `json:"data"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	Total      int            `json:"total"`
	TotalPages int            `json:"total_pages"`
}

type createCardRequest struct {
	Name            string  `json:"name" binding:"required"`
	ScryfallID      string  `json:"scryfall_id" binding:"required,uuid"`
	SetCode         string  `json:"set_code" binding:"required"`
	CollectorNumber string  `json:"collector_number" binding:"required"`
	Foil            bool    `json:"foil"`
	StorageID       *int    `json:"storage_id" binding:"omitempty,gt=0"`
	ManaValue       float64 `json:"mana_value" binding:"omitempty,gte=0"`
}

func (r createCardRequest) toDomain() Card {
	return Card{
		Name:            r.Name,
		ScryfallID:      r.ScryfallID,
		SetCode:         r.SetCode,
		CollectorNumber: r.CollectorNumber,
		Foil:            r.Foil,
		StorageID:       r.StorageID,
		ManaValue:       r.ManaValue,
	}
}

type updateCardRequest struct {
	Name            *string           `json:"name" binding:"omitempty"`
	ScryfallID      *string           `json:"scryfall_id" binding:"omitempty,uuid"`
	SetCode         *string           `json:"set_code" binding:"omitempty"`
	CollectorNumber *string           `json:"collector_number" binding:"omitempty"`
	Foil            *bool             `json:"foil" binding:"omitempty"`
	StorageID       optionalStorageID `json:"storage_id"`
	ManaValue       *float64          `json:"mana_value" binding:"omitempty,gte=0"`
}

type optionalStorageID struct {
	Set   bool
	Value *int
}

func (o *optionalStorageID) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}

	var id int
	if err := json.Unmarshal(data, &id); err != nil {
		return err
	}
	o.Value = &id
	return nil
}

var errInvalidStorageID = errors.New("storage_id must be a positive integer, or null to remove the card from its storage")

func (r updateCardRequest) validate() error {
	if r.StorageID.Value != nil && *r.StorageID.Value <= 0 {
		return errInvalidStorageID
	}
	return nil
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
	if r.ManaValue != nil {
		c.ManaValue = *r.ManaValue
	}
	if r.StorageID.Set {
		c.StorageID = r.StorageID.Value
	}
	return c
}

type cardService interface {
	GetAllCards(ctx context.Context, userID string, filter CardFilter) ([]Card, int, error)
	GetCard(ctx context.Context, userID string, id int) (Card, error)
	CreateCard(ctx context.Context, userID string, c Card) (Card, error)
	UpdateCard(ctx context.Context, userID string, id int, req updateCardRequest) (Card, error)
	DeleteCard(ctx context.Context, userID string, id int) error
	DeleteAllCards(ctx context.Context, userID string) (int, error)
}

type deleteAllCardsResponse struct {
	Deleted int `json:"deleted"`
}

type Handler struct {
	service cardService
}

func NewHandler(service cardService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/cards", h.getCards)
	router.GET("/cards/:id", h.getCard)
	router.POST("/cards", h.createCard)
	router.PATCH("/cards/:id", h.updateCard)
	router.DELETE("/cards/:id", h.deleteCard)
	router.DELETE("/cards", h.deleteAllCards)
}

func (h *Handler) getCards(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var storageID *int
	if raw := ctx.Query("storage_id"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id must be a valid integer"})
			return
		}
		storageID = &parsed
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
		case "name", "added", "updated", "mana_value":
			sortField = field
			sortDesc = desc
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "sort must be one of: name, -name, added, -added, updated, -updated, mana_value, -mana_value"})
			return
		}
	}

	filter := CardFilter{
		StorageID: storageID,
		Name:      ctx.Query("name"),
		SortField: sortField,
		SortDesc:  sortDesc,
		Page:      page,
		Limit:     limit,
	}

	cards, total, err := h.service.GetAllCards(ctx.Request.Context(), userID, filter)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}

	response := make([]cardResponse, 0, len(cards))
	for _, cd := range cards {
		response = append(response, toResponse(cd))
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	ctx.IndentedJSON(http.StatusOK, paginatedCardsResponse{
		Data:       response,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	})
}

func (h *Handler) getCard(ctx *gin.Context) {
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

	card, err := h.service.GetCard(ctx.Request.Context(), userID, id)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "card not found"})
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(card))
}

func (h *Handler) createCard(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var req createCardRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.service.CreateCard(ctx.Request.Context(), userID, req.toDomain())
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"})
		return
	}

	ctx.IndentedJSON(http.StatusCreated, toResponse(created))
}

func (h *Handler) updateCard(ctx *gin.Context) {
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

	var req updateCardRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.service.UpdateCard(ctx.Request.Context(), userID, id, req)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "card not found"},
			apierr.Mapping{Err: ErrStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"},
		)
		return
	}

	ctx.IndentedJSON(http.StatusOK, toResponse(updated))
}

func (h *Handler) deleteCard(ctx *gin.Context) {
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

	if err := h.service.DeleteCard(ctx.Request.Context(), userID, id); err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "card not found"})
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (h *Handler) deleteAllCards(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	if ctx.Query("confirm") != "true" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "confirm=true is required to delete every card"})
		return
	}

	deleted, err := h.service.DeleteAllCards(ctx.Request.Context(), userID)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, deleteAllCardsResponse{Deleted: deleted})
}
