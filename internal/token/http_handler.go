package token

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/authcookie"
)

type refreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

var errMissingRefreshToken = errors.New("refresh token is required (JSON body or cookie)")

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
	cookie         authcookie.RefreshCookie
}

func NewHandler(service tokenService, jwtSecret string, accessTokenTTL time.Duration, cookie authcookie.RefreshCookie) *Handler {
	return &Handler{service: service, jwtSecret: jwtSecret, accessTokenTTL: accessTokenTTL, cookie: cookie}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.POST("/auth/refresh", h.refresh)
	router.POST("/auth/logout", h.logout)
}

func (h *Handler) refresh(ctx *gin.Context) {
	refreshToken, err := h.refreshTokenFrom(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, newRefreshToken, err := h.service.Rotate(ctx.Request.Context(), refreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			h.cookie.Clear(ctx.Writer)
		}
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrInvalid, Status: http.StatusUnauthorized, Message: "invalid or expired refresh token"})
		return
	}

	accessToken, err := auth.GenerateToken(h.jwtSecret, userID, h.accessTokenTTL)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}

	h.cookie.Set(ctx.Writer, newRefreshToken)
	ctx.JSON(http.StatusOK, refreshResponse{Token: accessToken, RefreshToken: newRefreshToken})
}

func (h *Handler) logout(ctx *gin.Context) {
	h.cookie.Clear(ctx.Writer)

	refreshToken, err := h.refreshTokenFrom(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.Revoke(ctx.Request.Context(), refreshToken); err != nil {
		apierr.Respond(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (h *Handler) refreshTokenFrom(ctx *gin.Context) (string, error) {
	var req refreshTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	if req.RefreshToken != "" {
		return req.RefreshToken, nil
	}

	if fromCookie := h.cookie.Read(ctx.Request); fromCookie != "" {
		return fromCookie, nil
	}

	return "", errMissingRefreshToken
}
