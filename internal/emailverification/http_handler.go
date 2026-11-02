package emailverification

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/user"
)

type verifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

type verifier interface {
	Verify(ctx context.Context, plaintext string) error
	IssueAndSend(ctx context.Context, userID, email string) error
}

type accountLookup interface {
	GetUser(ctx context.Context, userID string) (user.User, error)
}

type Handler struct {
	service verifier
	users   accountLookup
}

func NewHandler(service verifier, users accountLookup) *Handler {
	return &Handler{service: service, users: users}
}

// RegisterProtectedRoutes mounts the authenticated routes; authMiddleware (the rate limiter) runs before the handler.
func (h *Handler) RegisterProtectedRoutes(router gin.IRoutes, authMiddleware ...gin.HandlerFunc) {
	router.POST("/auth/verify-email/resend", append(append([]gin.HandlerFunc{}, authMiddleware...), h.resend)...)
}

func (h *Handler) resend(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
		return
	}

	account, err := h.users.GetUser(ctx.Request.Context(), userID)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: user.ErrNotFound, Status: http.StatusNotFound, Message: "user not found"})
		return
	}
	if account.EmailVerifiedAt != nil {
		ctx.JSON(http.StatusConflict, gin.H{"error": "email already verified"})
		return
	}

	if err := h.service.IssueAndSend(ctx.Request.Context(), userID, account.Email); err != nil {
		apierr.Respond(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
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
