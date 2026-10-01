package user

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

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
	Token string `json:"token"`
}

type userService interface {
	Register(ctx context.Context, email, password string) (User, error)
	Authenticate(ctx context.Context, email, password string) (User, error)
	ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error
}

type Handler struct {
	service   userService
	jwtSecret string
	tokenTTL  time.Duration
}

func NewHandler(service userService, jwtSecret string) *Handler {
	return &Handler{
		service:   service,
		jwtSecret: jwtSecret,
		tokenTTL:  7 * 24 * time.Hour, // 7 jours
	}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware ...gin.HandlerFunc) {
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
		if errors.Is(err, ErrEmailAlreadyTaken) {
			ctx.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	token, err := auth.GenerateToken(h.jwtSecret, created.ID, h.tokenTTL)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, authResponse{Token: token})
}

func (h *Handler) login(ctx *gin.Context) {
	var req loginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	authenticated, err := h.service.Authenticate(ctx.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	token, err := auth.GenerateToken(h.jwtSecret, authenticated.ID, h.tokenTTL)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, authResponse{Token: token})
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

	err := h.service.ChangePassword(ctx.Request.Context(), userID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		if errors.Is(err, ErrIncorrectPassword) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect current password"})
			return
		}
		if errors.Is(err, ErrNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}
