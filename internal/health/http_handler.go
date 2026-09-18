package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type pinger interface {
	PingContext(ctx context.Context) error
}

type Handler struct {
	db pinger
}

func NewHandler(db pinger) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/health", h.check)
}

func (h *Handler) check(ctx *gin.Context) {
	pingCtx, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.PingContext(pingCtx); err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"error":  err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}
