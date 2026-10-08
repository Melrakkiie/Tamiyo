package deck

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
)

type viewResponse struct {
	Grouping *string `json:"grouping"`
	Sort     string  `json:"sort"`
}

type setViewRequest struct {
	Grouping *string `json:"grouping"`
	Sort     string  `json:"sort" binding:"required"`
}

func (h *Handler) registerViewRoutes(router gin.IRoutes) {
	router.GET("/deck/:id/view", h.getView)
	router.PUT("/deck/:id/view", h.setView)
}

func respondViewError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err,
		apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
		apierr.Mapping{Err: ErrInvalidViewGrouping, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrInvalidViewSort, Status: http.StatusBadRequest},
	)
}

func (h *Handler) getView(ctx *gin.Context) {
	userID, deckID, ok := h.deckRequestContext(ctx)
	if !ok {
		return
	}
	v, err := h.service.GetView(ctx.Request.Context(), userID, deckID)
	if err != nil {
		respondViewError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, viewResponse(v))
}

func (h *Handler) setView(ctx *gin.Context) {
	userID, deckID, ok := h.deckRequestContext(ctx)
	if !ok {
		return
	}
	var req setViewRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "sort is required"})
		return
	}
	v, err := h.service.SetView(ctx.Request.Context(), userID, deckID, View(req))
	if err != nil {
		respondViewError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, viewResponse(v))
}
