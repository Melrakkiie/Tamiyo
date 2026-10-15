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

const (
	CardDisplayText   = "text"
	CardDisplayImages = "images"
	CardDisplayGrid   = "grid"
	CardDisplayStacks = "stacks"

	MinCardColumns = 2
	MaxCardColumns = 12

	ThemeDay   = "day"
	ThemeNight = "night"
	ThemeAuto  = "auto"

	CurrencyEUR = "eur"
	CurrencyUSD = "usd"

	MinPlaytestZoom     = 50
	MaxPlaytestZoom     = 200
	DefaultPlaytestZoom = 100
)

const DefaultCollectionSort = "-added"

var (
	ErrInvalidCardDisplay        = errors.New("card_display must be one of: text, images, grid, stacks")
	ErrInvalidCardColumns        = errors.New("card_columns must be between 2 and 12")
	ErrInvalidCollectionGrouping = errors.New("collection_grouping must be one of: type, color, mana, subtype")
	ErrInvalidTheme              = errors.New("theme must be one of: day, night, auto")
	ErrInvalidNightHour          = errors.New("night_starts_at and night_ends_at must be hours between 0 and 23")
	ErrEmptyNight                = errors.New("night_starts_at and night_ends_at must differ")
	ErrInvalidPriceCurrency      = errors.New("price_currency must be one of: eur, usd")
	ErrInvalidPlaytestZoom       = errors.New("playtest_zoom must be between 50 and 200")
	ErrInvalidCollectionSort     = errors.New("collection_sort must be one of: name, -name, added, -added, updated, -updated, mana_value, -mana_value, color, -color, type, -type, price, -price")
)

var collectionGroupings = map[string]bool{"type": true, "color": true, "mana": true, "subtype": true}

var collectionSorts = map[string]bool{
	"name": true, "-name": true, "added": true, "-added": true, "updated": true, "-updated": true,
	"mana_value": true, "-mana_value": true, "color": true, "-color": true, "type": true, "-type": true,
	"price": true, "-price": true,
}

func ValidCardDisplay(display string) bool {
	return display == CardDisplayText || display == CardDisplayImages || display == CardDisplayGrid || display == CardDisplayStacks
}

func ValidCardColumns(columns int) bool {
	return columns >= MinCardColumns && columns <= MaxCardColumns
}

func validTheme(theme string) bool {
	return theme == ThemeDay || theme == ThemeNight || theme == ThemeAuto
}

func validCurrency(currency string) bool {
	return currency == CurrencyEUR || currency == CurrencyUSD
}

func validPlaytestZoom(zoom int) bool {
	return zoom >= MinPlaytestZoom && zoom <= MaxPlaytestZoom
}

func validHour(hour int) bool {
	return hour >= 0 && hour <= 23
}

type Preferences struct {
	ShowCollectionInDecks bool
	CardDisplay           string
	CardColumns           int
	ShowManaCost          bool
	ShowCardDetails       bool
	CollectionGrouping    *string
	CollectionSort        string
	Theme                 string
	NightStartsAt         int
	NightEndsAt           int
	ShowPrices            bool
	PriceCurrency         string
	PlaytestZoom          int
}

func Defaults() Preferences {
	return Preferences{
		ShowCollectionInDecks: true,
		CardDisplay:           CardDisplayImages,
		CardColumns:           6,
		ShowManaCost:          true,
		ShowCardDetails:       true,
		CollectionSort:        DefaultCollectionSort,
		Theme:                 ThemeAuto,
		NightStartsAt:         22,
		NightEndsAt:           6,
		PriceCurrency:         CurrencyEUR,
		PlaytestZoom:          DefaultPlaytestZoom,
	}
}

type Changes struct {
	ShowCollectionInDecks   *bool
	CardDisplay             *string
	CardColumns             *int
	ShowManaCost            *bool
	ShowCardDetails         *bool
	CollectionGrouping      *string
	ClearCollectionGrouping bool
	CollectionSort          *string
	Theme                   *string
	NightStartsAt           *int
	NightEndsAt             *int
	ShowPrices              *bool
	PriceCurrency           *string
	PlaytestZoom            *int
}

func (c Changes) empty() bool {
	return c.ShowCollectionInDecks == nil && c.CardDisplay == nil && c.CardColumns == nil && c.ShowManaCost == nil &&
		c.ShowCardDetails == nil && c.CollectionGrouping == nil && !c.ClearCollectionGrouping && c.CollectionSort == nil &&
		c.Theme == nil && c.NightStartsAt == nil && c.NightEndsAt == nil && c.ShowPrices == nil && c.PriceCurrency == nil &&
		c.PlaytestZoom == nil
}

func (c Changes) validate() error {
	if c.CardDisplay != nil && !ValidCardDisplay(*c.CardDisplay) {
		return ErrInvalidCardDisplay
	}
	if c.CardColumns != nil && !ValidCardColumns(*c.CardColumns) {
		return ErrInvalidCardColumns
	}
	if c.CollectionGrouping != nil && !collectionGroupings[*c.CollectionGrouping] {
		return ErrInvalidCollectionGrouping
	}
	if c.CollectionSort != nil && !collectionSorts[*c.CollectionSort] {
		return ErrInvalidCollectionSort
	}
	if c.Theme != nil && !validTheme(*c.Theme) {
		return ErrInvalidTheme
	}
	if (c.NightStartsAt != nil && !validHour(*c.NightStartsAt)) || (c.NightEndsAt != nil && !validHour(*c.NightEndsAt)) {
		return ErrInvalidNightHour
	}
	if c.PriceCurrency != nil && !validCurrency(*c.PriceCurrency) {
		return ErrInvalidPriceCurrency
	}
	if c.PlaytestZoom != nil && !validPlaytestZoom(*c.PlaytestZoom) {
		return ErrInvalidPlaytestZoom
	}
	return nil
}

func (c Changes) applyTo(p Preferences) Preferences {
	if c.ShowCollectionInDecks != nil {
		p.ShowCollectionInDecks = *c.ShowCollectionInDecks
	}
	if c.CardDisplay != nil {
		p.CardDisplay = *c.CardDisplay
	}
	if c.CardColumns != nil {
		p.CardColumns = *c.CardColumns
	}
	if c.ShowManaCost != nil {
		p.ShowManaCost = *c.ShowManaCost
	}
	if c.ShowCardDetails != nil {
		p.ShowCardDetails = *c.ShowCardDetails
	}
	if c.ClearCollectionGrouping {
		p.CollectionGrouping = nil
	} else if c.CollectionGrouping != nil {
		grouping := *c.CollectionGrouping
		p.CollectionGrouping = &grouping
	}
	if c.CollectionSort != nil {
		p.CollectionSort = *c.CollectionSort
	}
	if c.Theme != nil {
		p.Theme = *c.Theme
	}
	if c.NightStartsAt != nil {
		p.NightStartsAt = *c.NightStartsAt
	}
	if c.NightEndsAt != nil {
		p.NightEndsAt = *c.NightEndsAt
	}
	if c.ShowPrices != nil {
		p.ShowPrices = *c.ShowPrices
	}
	if c.PriceCurrency != nil {
		p.PriceCurrency = *c.PriceCurrency
	}
	if c.PlaytestZoom != nil {
		p.PlaytestZoom = *c.PlaytestZoom
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
	if err := changes.validate(); err != nil {
		return Preferences{}, err
	}
	current, err := s.Get(ctx, userID)
	if err != nil {
		return Preferences{}, err
	}
	updated := changes.applyTo(current)
	if updated.NightStartsAt == updated.NightEndsAt {
		return Preferences{}, ErrEmptyNight
	}
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
	err := r.db.QueryRowContext(ctx, `
		SELECT show_collection_in_decks, card_display, card_columns, show_mana_cost, show_card_details,
			collection_grouping, collection_sort, theme, night_starts_at, night_ends_at, show_prices, price_currency, playtest_zoom
		FROM tamiyo.user_preferences WHERE user_id = $1
	`, userID).Scan(&p.ShowCollectionInDecks, &p.CardDisplay, &p.CardColumns, &p.ShowManaCost, &p.ShowCardDetails,
		&p.CollectionGrouping, &p.CollectionSort, &p.Theme, &p.NightStartsAt, &p.NightEndsAt, &p.ShowPrices, &p.PriceCurrency, &p.PlaytestZoom)
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
		INSERT INTO tamiyo.user_preferences (user_id, show_collection_in_decks, card_display, card_columns, show_mana_cost, show_card_details,
			collection_grouping, collection_sort, theme, night_starts_at, night_ends_at, show_prices, price_currency, playtest_zoom)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (user_id) DO UPDATE SET
			show_collection_in_decks = EXCLUDED.show_collection_in_decks,
			card_display = EXCLUDED.card_display,
			card_columns = EXCLUDED.card_columns,
			show_mana_cost = EXCLUDED.show_mana_cost,
			show_card_details = EXCLUDED.show_card_details,
			collection_grouping = EXCLUDED.collection_grouping,
			collection_sort = EXCLUDED.collection_sort,
			theme = EXCLUDED.theme,
			night_starts_at = EXCLUDED.night_starts_at,
			night_ends_at = EXCLUDED.night_ends_at,
			show_prices = EXCLUDED.show_prices,
			price_currency = EXCLUDED.price_currency,
			playtest_zoom = EXCLUDED.playtest_zoom
	`, userID, p.ShowCollectionInDecks, p.CardDisplay, p.CardColumns, p.ShowManaCost, p.ShowCardDetails,
		p.CollectionGrouping, p.CollectionSort, p.Theme, p.NightStartsAt, p.NightEndsAt, p.ShowPrices, p.PriceCurrency, p.PlaytestZoom)
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
	ShowCollectionInDecks bool    `json:"show_collection_in_decks"`
	CardDisplay           string  `json:"card_display"`
	CardColumns           int     `json:"card_columns"`
	ShowManaCost          bool    `json:"show_mana_cost"`
	ShowCardDetails       bool    `json:"show_card_details"`
	CollectionGrouping    *string `json:"collection_grouping"`
	CollectionSort        string  `json:"collection_sort"`
	Theme                 string  `json:"theme"`
	NightStartsAt         int     `json:"night_starts_at"`
	NightEndsAt           int     `json:"night_ends_at"`
	ShowPrices            bool    `json:"show_prices"`
	PriceCurrency         string  `json:"price_currency"`
	PlaytestZoom          int     `json:"playtest_zoom"`
}

type updateRequest struct {
	ShowCollectionInDecks   *bool   `json:"show_collection_in_decks"`
	CardDisplay             *string `json:"card_display"`
	CardColumns             *int    `json:"card_columns"`
	ShowManaCost            *bool   `json:"show_mana_cost"`
	ShowCardDetails         *bool   `json:"show_card_details"`
	CollectionGrouping      *string `json:"collection_grouping"`
	ClearCollectionGrouping bool    `json:"clear_collection_grouping"`
	CollectionSort          *string `json:"collection_sort"`
	Theme                   *string `json:"theme"`
	NightStartsAt           *int    `json:"night_starts_at"`
	NightEndsAt             *int    `json:"night_ends_at"`
	ShowPrices              *bool   `json:"show_prices"`
	PriceCurrency           *string `json:"price_currency"`
	PlaytestZoom            *int    `json:"playtest_zoom"`
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
	if Changes(req).empty() {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "give at least one preference to change"})
		return
	}
	p, err := h.service.Update(ctx.Request.Context(), userID, Changes(req))
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrInvalidCardDisplay, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrInvalidCardColumns, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrInvalidCollectionGrouping, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrInvalidCollectionSort, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrInvalidTheme, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrInvalidNightHour, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrEmptyNight, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrInvalidPriceCurrency, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrInvalidPlaytestZoom, Status: http.StatusBadRequest},
		)
		return
	}
	ctx.JSON(http.StatusOK, response(p))
}
