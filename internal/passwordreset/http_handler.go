package passwordreset

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
)

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

type resetService interface {
	Issue(ctx context.Context, userID string) (string, error)
	Consume(ctx context.Context, plaintext string) (userID string, err error)
}

type userLookup interface {
	FindIDByEmail(ctx context.Context, email string) (string, error)
	SetPassword(ctx context.Context, userID, newPassword string) error
}

type sessionRevoker interface {
	RevokeAllForUser(ctx context.Context, userID string) error
}

type emailSender interface {
	SendPasswordResetEmail(ctx context.Context, toEmail, resetToken string) error
}

type Handler struct {
	service resetService
	users   userLookup
	tokens  sessionRevoker
	mailer  emailSender
}

func NewHandler(service resetService, users userLookup, tokens sessionRevoker, mailer emailSender) *Handler {
	return &Handler{service: service, users: users, tokens: tokens, mailer: mailer}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes, authMiddleware ...gin.HandlerFunc) {
	forgotHandlers := append(append([]gin.HandlerFunc{}, authMiddleware...), h.forgotPassword)
	resetHandlers := append(append([]gin.HandlerFunc{}, authMiddleware...), h.resetPassword)

	router.POST("/auth/forgot-password", forgotHandlers...)
	router.POST("/auth/reset-password", resetHandlers...)
}

func (h *Handler) forgotPassword(ctx *gin.Context) {
	var req forgotPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if userID, err := h.users.FindIDByEmail(ctx.Request.Context(), req.Email); err == nil {
		if token, err := h.service.Issue(ctx.Request.Context(), userID); err == nil {
			_ = h.mailer.SendPasswordResetEmail(ctx.Request.Context(), req.Email, token)
		}
	}

	ctx.Status(http.StatusNoContent)
}

func (h *Handler) resetPassword(ctx *gin.Context) {
	var req resetPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, err := h.service.Consume(ctx.Request.Context(), req.Token)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrInvalid, Status: http.StatusUnauthorized, Message: "invalid or expired reset token"})
		return
	}

	if err := h.users.SetPassword(ctx.Request.Context(), userID, req.NewPassword); err != nil {
		apierr.Respond(ctx, err)
		return
	}

	if err := h.tokens.RevokeAllForUser(ctx.Request.Context(), userID); err != nil {
		apierr.Respond(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}
