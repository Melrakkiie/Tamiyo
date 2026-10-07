package emailchange

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/user"
)

type requestEmailChangeRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewEmail        string `json:"new_email" binding:"required,email"`
}

type confirmEmailChangeRequest struct {
	Token string `json:"token" binding:"required"`
}

type changeService interface {
	Issue(ctx context.Context, userID, newEmail string) (string, error)
	Consume(ctx context.Context, plaintext string) (userID, newEmail string, err error)
}

type userAccounts interface {
	CheckPassword(ctx context.Context, userID, password string) (string, error)
	EmailTaken(ctx context.Context, email string) (bool, error)
	ChangeEmail(ctx context.Context, userID, newEmail string) (string, error)
}

type emailSender interface {
	SendEmailChangeConfirmation(ctx context.Context, toEmail, newEmail, token string) error
	SendEmailChangedNotice(ctx context.Context, toEmail string) error
}

type Handler struct {
	service changeService
	users   userAccounts
	mailer  emailSender
}

func NewHandler(service changeService, users userAccounts, mailer emailSender) *Handler {
	return &Handler{service: service, users: users, mailer: mailer}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes, authMiddleware ...gin.HandlerFunc) {
	confirmHandlers := append(append([]gin.HandlerFunc{}, authMiddleware...), h.confirmEmailChange)
	router.POST("/auth/confirm-email", confirmHandlers...)
}

func (h *Handler) RegisterProtectedRoutes(router gin.IRoutes) {
	router.POST("/auth/email", h.requestEmailChange)
}

var emailTaken = apierr.Mapping{Err: user.ErrEmailAlreadyTaken, Status: http.StatusConflict, Message: "email already registered"}

func (h *Handler) requestEmailChange(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
		return
	}

	var req requestEmailChangeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	newEmail := strings.TrimSpace(req.NewEmail)

	currentEmail, err := h.users.CheckPassword(ctx.Request.Context(), userID, req.CurrentPassword)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: user.ErrIncorrectPassword, Status: http.StatusUnauthorized, Message: "incorrect current password"},
			apierr.Mapping{Err: user.ErrNotFound, Status: http.StatusNotFound, Message: "user not found"},
		)
		return
	}

	if strings.EqualFold(currentEmail, newEmail) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": ErrSameEmail.Error()})
		return
	}

	taken, err := h.users.EmailTaken(ctx.Request.Context(), newEmail)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}
	if taken {
		apierr.Respond(ctx, user.ErrEmailAlreadyTaken, emailTaken)
		return
	}

	token, err := h.service.Issue(ctx.Request.Context(), userID, newEmail)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}

	if err := h.mailer.SendEmailChangeConfirmation(ctx.Request.Context(), currentEmail, newEmail, token); err != nil {
		apierr.Respond(ctx, err)
		return
	}

	ctx.Status(http.StatusAccepted)
}

func (h *Handler) confirmEmailChange(ctx *gin.Context) {
	var req confirmEmailChangeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, newEmail, err := h.service.Consume(ctx.Request.Context(), req.Token)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrInvalid, Status: http.StatusUnauthorized, Message: "invalid or expired email change token"})
		return
	}

	if _, err := h.users.ChangeEmail(ctx.Request.Context(), userID, newEmail); err != nil {
		apierr.Respond(ctx, err, emailTaken,
			apierr.Mapping{Err: user.ErrNotFound, Status: http.StatusUnauthorized, Message: "invalid or expired email change token"},
		)
		return
	}

	_ = h.mailer.SendEmailChangedNotice(ctx.Request.Context(), newEmail)

	ctx.Status(http.StatusNoContent)
}
