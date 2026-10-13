package deck

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
)

type viewResponse struct {
	Grouping        *string  `json:"grouping"`
	Sort            string   `json:"sort"`
	CollapsedBoards []string `json:"collapsed_boards"`
	CardDisplay     string   `json:"card_display"`
	CardColumns     int      `json:"card_columns"`
	ShowManaCost    bool     `json:"show_mana_cost"`
	ShowCardDetails bool     `json:"show_card_details"`
}

func toViewResponse(v View) viewResponse {
	boards := v.CollapsedBoards
	if boards == nil {
		boards = []string{}
	}
	v.CollapsedBoards = boards
	return viewResponse(v)
}

type setViewRequest struct {
	Grouping        *string  `json:"grouping"`
	Sort            string   `json:"sort" binding:"required"`
	CollapsedBoards []string `json:"collapsed_boards"`
	CardDisplay     *string  `json:"card_display"`
	CardColumns     *int     `json:"card_columns"`
	ShowManaCost    *bool    `json:"show_mana_cost"`
	ShowCardDetails *bool    `json:"show_card_details"`
}

func (r setViewRequest) displaySet() bool {
	return r.CardDisplay != nil && r.CardColumns != nil && r.ShowManaCost != nil && r.ShowCardDetails != nil
}

func (r setViewRequest) toView(current View) View {
	v := View{
		Grouping:        r.Grouping,
		Sort:            r.Sort,
		CollapsedBoards: r.CollapsedBoards,
		CardDisplay:     current.CardDisplay,
		CardColumns:     current.CardColumns,
		ShowManaCost:    current.ShowManaCost,
		ShowCardDetails: current.ShowCardDetails,
	}
	if r.CardDisplay != nil {
		v.CardDisplay = *r.CardDisplay
	}
	if r.CardColumns != nil {
		v.CardColumns = *r.CardColumns
	}
	if r.ShowManaCost != nil {
		v.ShowManaCost = *r.ShowManaCost
	}
	if r.ShowCardDetails != nil {
		v.ShowCardDetails = *r.ShowCardDetails
	}
	return v
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
		apierr.Mapping{Err: ErrInvalidViewBoards, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrInvalidViewDisplay, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrInvalidViewColumns, Status: http.StatusBadRequest},
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
	ctx.JSON(http.StatusOK, toViewResponse(v))
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
	current := DefaultView()
	if !req.displaySet() {
		saved, err := h.service.GetView(ctx.Request.Context(), userID, deckID)
		if err != nil {
			respondViewError(ctx, err)
			return
		}
		current = saved
	}
	v, err := h.service.SetView(ctx.Request.Context(), userID, deckID, req.toView(current))
	if err != nil {
		respondViewError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, toViewResponse(v))
}
