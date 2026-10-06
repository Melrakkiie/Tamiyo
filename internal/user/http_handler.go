package user

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

type authResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type userService interface {
	Register(ctx context.Context, email, password string) (User, error)
	Authenticate(ctx context.Context, email, password string) (User, error)
	ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error
}

type refreshTokenService interface {
	IssueRefreshToken(ctx context.Context, userID string) (string, error)
	RevokeAllForUser(ctx context.Context, userID string) error
}

type Handler struct {
	service        userService
	jwtSecret      string
	accessTokenTTL time.Duration
	tokens         refreshTokenService
}

func NewHandler(service userService, jwtSecret string, accessTokenTTL time.Duration, tokens refreshTokenService) *Handler {
	return &Handler{
		service:        service,
		jwtSecret:      jwtSecret,
		accessTokenTTL: accessTokenTTL,
		tokens:         tokens,
	}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes, authMiddleware ...gin.HandlerFunc) {
	registerHandlers := append(append([]gin.HandlerFunc{}, authMiddleware...), h.register)
	loginHandlers := append(append([]gin.HandlerFunc{}, authMiddleware...), h.login)

	router.POST("/auth/register", registerHandlers...)
	router.POST("/auth/login", loginHandlers...)
}

func (h *Handler) RegisterProtectedRoutes(router gin.IRoutes) {
	router.POST("/auth/password", h.changePassword)
}

func (h *Handler) register(ctx *gin.Context) {
	var req registerRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.service.Register(ctx.Request.Context(), req.Email, req.Password)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrEmailAlreadyTaken, Status: http.StatusConflict, Message: "email already registered"})
		return
	}

	h.respondWithTokenPair(ctx, created.ID, http.StatusCreated)
}

func (h *Handler) login(ctx *gin.Context) {
	var req loginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	authenticated, err := h.service.Authenticate(ctx.Request.Context(), req.Email, req.Password)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrInvalidCredentials, Status: http.StatusUnauthorized, Message: "invalid email or password"})
		return
	}

	h.respondWithTokenPair(ctx, authenticated.ID, http.StatusOK)
}

func (h *Handler) respondWithTokenPair(ctx *gin.Context, userID string, status int) {
	accessToken, err := auth.GenerateToken(h.jwtSecret, userID, h.accessTokenTTL)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}

	refreshToken, err := h.tokens.IssueRefreshToken(ctx.Request.Context(), userID)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}

	ctx.JSON(status, authResponse{Token: accessToken, RefreshToken: refreshToken})
}

func (h *Handler) changePassword(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
		return
	}

	var req changePasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.ChangePassword(ctx.Request.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrIncorrectPassword, Status: http.StatusUnauthorized, Message: "incorrect current password"},
			apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "user not found"},
		)
		return
	}

	if err := h.tokens.RevokeAllForUser(ctx.Request.Context(), userID); err != nil {
		apierr.Respond(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}
