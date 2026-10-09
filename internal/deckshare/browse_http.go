package deckshare

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/deck"
)

const (
	defaultBrowseLimit = 24
	maxBrowseLimit     = 100
	maxBrowseText      = 100
)

type publicDeckResponse struct {
	ID                   string        `json:"id"`
	Name                 string        `json:"name"`
	Format               string        `json:"format"`
	BackgroundScryfallID *string       `json:"background_scryfall_id"`
	CommanderScryfallID  *string       `json:"commander_scryfall_id"`
	CommanderName        *string       `json:"commander_name"`
	ColorIdentity        string        `json:"color_identity"`
	CardCount            int           `json:"card_count"`
	Owner                ownerResponse `json:"owner"`
	Added                string        `json:"added"`
	Updated              string        `json:"updated"`
}

type publicDecksResponse struct {
	Data       []publicDeckResponse `json:"data"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
	Total      int                  `json:"total"`
	TotalPages int                  `json:"total_pages"`
}

func toPublicDeckResponse(d deck.PublicDeck) publicDeckResponse {
	return publicDeckResponse{
		ID:                   d.ID,
		Name:                 d.Name,
		Format:               d.Format,
		BackgroundScryfallID: d.BackgroundScryfallID,
		CommanderScryfallID:  d.CommanderScryfallID,
		CommanderName:        d.CommanderName,
		ColorIdentity:        d.ColorIdentity,
		CardCount:            d.CardCount,
		Owner:                ownerResponse{ID: d.OwnerID, DisplayName: d.OwnerDisplayName, AvatarScryfallID: d.OwnerAvatarID},
		Added:                d.Added.Format("2006-01-02 15:04:05"),
		Updated:              d.Updated.Format("2006-01-02 15:04:05"),
	}
}

var browseSorts = map[string]bool{"updated": true, "name": true, "added": true, "card_count": true}

type queryError string

func (e queryError) Error() string { return string(e) }

func positiveInt(ctx *gin.Context, name string, fallback int, max int) (int, error) {
	raw := ctx.Query(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || (max > 0 && value > max) {
		if max > 0 {
			return 0, queryError(fmt.Sprintf("%s must be an integer between 1 and %d", name, max))
		}
		return 0, queryError(name + " must be a positive integer")
	}
	return value, nil
}

func parseBrowseQuery(ctx *gin.Context) (deck.PublicFilter, error) {
	var filter deck.PublicFilter
	var err error
	if filter.Page, err = positiveInt(ctx, "page", 1, 0); err != nil {
		return filter, err
	}
	if filter.Limit, err = positiveInt(ctx, "limit", defaultBrowseLimit, maxBrowseLimit); err != nil {
		return filter, err
	}

	texts := map[string]*string{
		"q":         &filter.Name,
		"format":    &filter.Format,
		"commander": &filter.Commander,
		"card":      &filter.Card,
		"owner":     &filter.Owner,
	}
	for name, target := range texts {
		value := strings.TrimSpace(ctx.Query(name))
		if len([]rune(value)) > maxBrowseText {
			return filter, queryError(fmt.Sprintf("%s must be at most %d characters", name, maxBrowseText))
		}
		*target = value
	}

	if raw := strings.ToUpper(strings.TrimSpace(ctx.Query("colors"))); raw != "" {
		if raw == "C" {
			filter.Colorless = true
		} else {
			if strings.Trim(raw, "WUBRG") != "" {
				return filter, queryError("colors must only contain the letters W, U, B, R and G, or be C for colorless")
			}
			for _, letter := range strings.Split(raw, "") {
				if !contains(filter.Colors, letter) {
					filter.Colors = append(filter.Colors, letter)
				}
			}
		}
	}

	filter.ColorMode = deck.ColorModeExact
	if raw := ctx.Query("color_mode"); raw != "" {
		switch raw {
		case deck.ColorModeExact, deck.ColorModeInclude, deck.ColorModeWithin:
			filter.ColorMode = raw
		default:
			return filter, queryError("color_mode must be one of: exact, include, within")
		}
	}

	if raw := ctx.Query("color_count"); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count < 0 || count > 5 {
			return filter, queryError("color_count must be an integer between 0 and 5")
		}
		filter.ColorCount = &count
	}

	filter.SortField = "updated"
	filter.SortDesc = true
	if raw := ctx.Query("sort"); raw != "" {
		field := strings.TrimPrefix(raw, "-")
		if !browseSorts[field] {
			return filter, queryError("sort must be one of: updated, -updated, name, -name, added, -added, card_count, -card_count")
		}
		filter.SortField = field
		filter.SortDesc = strings.HasPrefix(raw, "-")
	}
	return filter, nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (h *Handler) browsePublicDecks(ctx *gin.Context) {
	filter, err := parseBrowseQuery(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	decks, total, err := h.service.BrowsePublicDecks(ctx.Request.Context(), filter)
	if err != nil {
		apierr.Respond(ctx, err)
		return
	}

	response := publicDecksResponse{
		Data:  make([]publicDeckResponse, 0, len(decks)),
		Page:  filter.Page,
		Limit: filter.Limit,
		Total: total,
	}
	for _, d := range decks {
		response.Data = append(response.Data, toPublicDeckResponse(d))
	}
	if total > 0 {
		response.TotalPages = (total + filter.Limit - 1) / filter.Limit
	}
	ctx.JSON(http.StatusOK, response)
}
