package follow

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

const (
	defaultLimit = 24
	maxLimit     = 100
)

var userIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type service interface {
	Status(ctx context.Context, viewerID string, userID string) (Status, error)
	Follow(ctx context.Context, viewerID string, userID string) (Status, error)
	Unfollow(ctx context.Context, viewerID string, userID string) (Status, error)
	Followers(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error)
	Following(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error)
}

type Handler struct {
	service service
}

func NewHandler(s service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/users/:id/follow", h.status)
	router.PUT("/users/:id/follow", h.follow)
	router.DELETE("/users/:id/follow", h.unfollow)
	router.GET("/users/:id/followers", h.followers)
	router.GET("/users/:id/following", h.following)
}

type statusResponse struct {
	Followers    int  `json:"followers_count"`
	Following    int  `json:"following_count"`
	FollowedByMe bool `json:"followed_by_me"`
	FollowsMe    bool `json:"follows_me"`
}

type connectionResponse struct {
	ID               string    `json:"id"`
	DisplayName      *string   `json:"display_name"`
	AvatarScryfallID *string   `json:"avatar_scryfall_id"`
	Since            time.Time `json:"since"`
	FollowedByMe     bool      `json:"followed_by_me"`
}

type connectionsResponse struct {
	Data       []connectionResponse `json:"data"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
	Total      int                  `json:"total"`
	TotalPages int                  `json:"total_pages"`
}

func (h *Handler) parties(ctx *gin.Context) (string, string, bool) {
	viewerID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return "", "", false
	}
	userID := ctx.Param("id")
	if !userIDPattern.MatchString(userID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return "", "", false
	}
	return viewerID, userID, true
}

func respondError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err,
		apierr.Mapping{Err: ErrUserNotFound, Status: http.StatusNotFound},
		apierr.Mapping{Err: ErrSelfFollow, Status: http.StatusBadRequest},
	)
}

func (h *Handler) respondStatus(ctx *gin.Context, change func(context.Context, string, string) (Status, error)) {
	viewerID, userID, ok := h.parties(ctx)
	if !ok {
		return
	}
	status, err := change(ctx.Request.Context(), viewerID, userID)
	if err != nil {
		respondError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, statusResponse(status))
}

func (h *Handler) status(ctx *gin.Context) {
	h.respondStatus(ctx, h.service.Status)
}

func (h *Handler) follow(ctx *gin.Context) {
	h.respondStatus(ctx, h.service.Follow)
}

func (h *Handler) unfollow(ctx *gin.Context) {
	h.respondStatus(ctx, h.service.Unfollow)
}

func queryInt(ctx *gin.Context, name string, fallback int, max int) (int, bool) {
	raw := ctx.Query(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || (max > 0 && value > max) {
		message := name + " must be a positive integer"
		if max > 0 {
			message = name + " must be an integer between 1 and " + strconv.Itoa(max)
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": message})
		return 0, false
	}
	return value, true
}

func (h *Handler) respondConnections(ctx *gin.Context, list func(context.Context, string, string, Page) ([]Connection, int, error)) {
	viewerID, userID, ok := h.parties(ctx)
	if !ok {
		return
	}
	var page Page
	if page.Number, ok = queryInt(ctx, "page", 1, 0); !ok {
		return
	}
	if page.Limit, ok = queryInt(ctx, "limit", defaultLimit, maxLimit); !ok {
		return
	}

	connections, total, err := list(ctx.Request.Context(), viewerID, userID, page)
	if err != nil {
		respondError(ctx, err)
		return
	}
	response := connectionsResponse{
		Data:       make([]connectionResponse, 0, len(connections)),
		Page:       page.Number,
		Limit:      page.Limit,
		Total:      total,
		TotalPages: (total + page.Limit - 1) / page.Limit,
	}
	for _, c := range connections {
		response.Data = append(response.Data, connectionResponse(c))
	}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) followers(ctx *gin.Context) {
	h.respondConnections(ctx, h.service.Followers)
}

func (h *Handler) following(ctx *gin.Context) {
	h.respondConnections(ctx, h.service.Following)
}
