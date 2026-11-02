package emailverification

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
)

type verifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

type verifier interface {
	Verify(ctx context.Context, plaintext string) error
}

type Handler struct {
	service verifier
}

func NewHandler(service verifier) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes mounts the public routes; authMiddleware (the rate limiter) runs before the handler.
func (h *Handler) RegisterRoutes(router gin.IRoutes, authMiddleware ...gin.HandlerFunc) {
	router.POST("/auth/verify-email", append(append([]gin.HandlerFunc{}, authMiddleware...), h.verifyEmail)...)
}

func (h *Handler) verifyEmail(ctx *gin.Context) {
	var req verifyEmailRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.Verify(ctx.Request.Context(), req.Token); err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrInvalid, Status: http.StatusBadRequest, Message: "invalid or expired verification token"})
		return
	}

	ctx.Status(http.StatusNoContent)
}
