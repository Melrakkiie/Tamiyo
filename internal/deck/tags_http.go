package deck

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

type taggedCardResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type deckTagsResponse struct {
	Tags  []string             `json:"tags"`
	Cards []taggedCardResponse `json:"cards"`
}

type setCardTagsRequest struct {
	Name string   `json:"name" binding:"required"`
	Tags []string `json:"tags" binding:"required"`
}

type renameTagRequest struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to" binding:"required"`
}

func (h *Handler) registerTagRoutes(router gin.IRoutes) {
	router.GET("/deck/:id/tags", h.getDeckTags)
	router.PUT("/deck/:id/tags/cards", h.setCardTags)
	router.PATCH("/deck/:id/tags", h.renameTag)
	router.DELETE("/deck/:id/tags", h.deleteTag)
}

func (h *Handler) deckRequestContext(ctx *gin.Context) (string, string, bool) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return "", "", false
	}
	deckID, valid := ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", "", false
	}
	return userID, deckID, true
}

func respondTagError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err,
		apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
		apierr.Mapping{Err: ErrCardNotInDeck, Status: http.StatusNotFound},
		apierr.Mapping{Err: ErrTagNotFound, Status: http.StatusNotFound},
		apierr.Mapping{Err: ErrInvalidTag, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrTooManyTags, Status: http.StatusBadRequest},
	)
}

func toTaggedCardResponse(c TaggedCard) taggedCardResponse {
	tags := c.Tags
	if tags == nil {
		tags = []string{}
	}
	return taggedCardResponse{Name: c.Name, Tags: tags}
}

func (h *Handler) getDeckTags(ctx *gin.Context) {
	userID, deckID, ok := h.deckRequestContext(ctx)
	if !ok {
		return
	}

	tags, err := h.service.GetCardTags(ctx.Request.Context(), userID, deckID)
	if err != nil {
		respondTagError(ctx, err)
		return
	}

	response := deckTagsResponse{Tags: tags.Tags, Cards: make([]taggedCardResponse, 0, len(tags.Cards))}
	if response.Tags == nil {
		response.Tags = []string{}
	}
	for _, c := range tags.Cards {
		response.Cards = append(response.Cards, toTaggedCardResponse(c))
	}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) setCardTags(ctx *gin.Context) {
	userID, deckID, ok := h.deckRequestContext(ctx)
	if !ok {
		return
	}

	var req setCardTagsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name and tags are required"})
		return
	}

	tagged, err := h.service.SetCardTags(ctx.Request.Context(), userID, deckID, req.Name, req.Tags)
	if err != nil {
		respondTagError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, toTaggedCardResponse(tagged))
}

func (h *Handler) renameTag(ctx *gin.Context) {
	userID, deckID, ok := h.deckRequestContext(ctx)
	if !ok {
		return
	}

	var req renameTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "from and to are required"})
		return
	}

	if err := h.service.RenameTag(ctx.Request.Context(), userID, deckID, req.From, req.To); err != nil {
		respondTagError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) deleteTag(ctx *gin.Context) {
	userID, deckID, ok := h.deckRequestContext(ctx)
	if !ok {
		return
	}

	tag := ctx.Query("tag")
	if tag == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "tag is required"})
		return
	}

	if err := h.service.DeleteTag(ctx.Request.Context(), userID, deckID, tag); err != nil {
		respondTagError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
