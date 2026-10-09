package preference

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

type Preferences struct {
	ShowCollectionInDecks bool
}

func Defaults() Preferences {
	return Preferences{ShowCollectionInDecks: true}
}

type Changes struct {
	ShowCollectionInDecks *bool
}

func (c Changes) applyTo(p Preferences) Preferences {
	if c.ShowCollectionInDecks != nil {
		p.ShowCollectionInDecks = *c.ShowCollectionInDecks
	}
	return p
}

type Repository interface {
	Find(ctx context.Context, userID string) (Preferences, bool, error)
	Save(ctx context.Context, userID string, p Preferences) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context, userID string) (Preferences, error) {
	p, found, err := s.repo.Find(ctx, userID)
	if err != nil {
		return Preferences{}, err
	}
	if !found {
		return Defaults(), nil
	}
	return p, nil
}

func (s *Service) Update(ctx context.Context, userID string, changes Changes) (Preferences, error) {
	current, err := s.Get(ctx, userID)
	if err != nil {
		return Preferences{}, err
	}
	updated := changes.applyTo(current)
	if err := s.repo.Save(ctx, userID, updated); err != nil {
		return Preferences{}, err
	}
	return updated, nil
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Find(ctx context.Context, userID string) (Preferences, bool, error) {
	var p Preferences
	err := r.db.QueryRowContext(ctx, `SELECT show_collection_in_decks FROM tamiyo.user_preferences WHERE user_id = $1`, userID).
		Scan(&p.ShowCollectionInDecks)
	if errors.Is(err, sql.ErrNoRows) {
		return Preferences{}, false, nil
	}
	if err != nil {
		return Preferences{}, false, err
	}
	return p, true, nil
}

func (r *PostgresRepository) Save(ctx context.Context, userID string, p Preferences) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamiyo.user_preferences (user_id, show_collection_in_decks)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET show_collection_in_decks = EXCLUDED.show_collection_in_decks
	`, userID, p.ShowCollectionInDecks)
	return err
}

type service interface {
	Get(ctx context.Context, userID string) (Preferences, error)
	Update(ctx context.Context, userID string, changes Changes) (Preferences, error)
}

type Handler struct {
	service service
}

func NewHandler(s service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/auth/me/preferences", h.get)
	router.PATCH("/auth/me/preferences", h.update)
}

type response struct {
	ShowCollectionInDecks bool `json:"show_collection_in_decks"`
}

type updateRequest struct {
	ShowCollectionInDecks *bool `json:"show_collection_in_decks"`
}

func (h *Handler) get(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	p, err := h.service.Get(ctx.Request.Context(), userID)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response(p))
}

func (h *Handler) update(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req updateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ShowCollectionInDecks == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "show_collection_in_decks is required"})
		return
	}
	p, err := h.service.Update(ctx.Request.Context(), userID, Changes(req))
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response(p))
}
