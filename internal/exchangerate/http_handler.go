package exchangerate

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
)

type currentRate interface {
	Current(ctx context.Context) (Rate, bool, error)
}

type Handler struct {
	service currentRate
}

func NewHandler(service currentRate) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/exchange-rate", h.get)
}

type response struct {
	USDPerEUR *float64 `json:"usd_per_eur"`
	Date      *string  `json:"date"`
}

func (h *Handler) get(ctx *gin.Context) {
	rate, found, err := h.service.Current(ctx.Request.Context())
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}
	if !found {
		ctx.JSON(http.StatusOK, response{})
		return
	}
	date := rate.Date.Format(time.DateOnly)
	ctx.JSON(http.StatusOK, response{USDPerEUR: &rate.PerEUR, Date: &date})
}
