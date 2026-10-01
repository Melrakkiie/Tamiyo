package token

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/auth"
)

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type refreshResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type tokenService interface {
	Rotate(ctx context.Context, plaintext string) (userID string, newPlaintext string, err error)
	Revoke(ctx context.Context, plaintext string) error
}

type Handler struct {
	service        tokenService
	jwtSecret      string
	accessTokenTTL time.Duration
}

func NewHandler(service tokenService, jwtSecret string, accessTokenTTL time.Duration) *Handler {
	return &Handler{service: service, jwtSecret: jwtSecret, accessTokenTTL: accessTokenTTL}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.POST("/auth/refresh", h.refresh)
	router.POST("/auth/logout", h.logout)
}

func (h *Handler) refresh(ctx *gin.Context) {
	var req refreshRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, newRefreshToken, err := h.service.Rotate(ctx.Request.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	accessToken, err := auth.GenerateToken(h.jwtSecret, userID, h.accessTokenTTL)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, refreshResponse{Token: accessToken, RefreshToken: newRefreshToken})
}

func (h *Handler) logout(ctx *gin.Context) {
	var req logoutRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.Revoke(ctx.Request.Context(), req.RefreshToken); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}
