package card

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

var (
	errInvalidFilterColors = errors.New("colors must only contain the letters W, U, B, R and G")
	errInvalidColorMode    = errors.New("color_mode must be one of: exact, include, within")
	errInvalidManaValue    = errors.New("mana_value must be a number between 0 and 1000000")
	errInvalidManaValueOp  = errors.New("mana_value_op must be one of: eq, lt, lte, gt, gte")
	errInvalidPower        = errors.New("power must be a number between -1000000 and 1000000")
	errInvalidPowerOp      = errors.New("power_op must be one of: eq, lt, lte, gt, gte")
	errInvalidToughness    = errors.New("toughness must be a number between -1000000 and 1000000")
	errInvalidToughnessOp  = errors.New("toughness_op must be one of: eq, lt, lte, gt, gte")
	errInvalidTypeFilter   = errors.New("type must be one of: Creature, Planeswalker, Battle, Instant, Sorcery, Artifact, Enchantment, Land")
	errInvalidLegalIn      = errors.New("legal_in must be a Scryfall format, such as commander or modern")
	errInvalidColorCount   = errors.New("color_count must be an integer between 0 and 5")
	errInvalidFoilFilter   = errors.New("foil must be true or false")
	errInvalidSubtype      = errors.New("subtype must have at most 50 characters")
	errInvalidStorageType  = errors.New("storage_type must have at most 50 characters")
	errInvalidOpenToDeck   = errors.New("open_to_deck must be a deck id")
)

func parseAdvancedFilters(ctx *gin.Context) (CardFilter, error) {
	var filter CardFilter

	if raw, present := ctx.GetQuery("colors"); present {
		upper := strings.ToUpper(raw)
		if strings.Trim(upper, "WUBRG") != "" {
			return filter, errInvalidFilterColors
		}
		normalized := scryfall.ColorCode(strings.Split(upper, ""))
		filter.Colors = &normalized
		filter.ColorMode = ColorModeExact
		if mode := ctx.Query("color_mode"); mode != "" {
			if mode != ColorModeExact && mode != ColorModeInclude && mode != ColorModeWithin {
				return filter, errInvalidColorMode
			}
			filter.ColorMode = mode
		}
	}

	var err error
	if filter.ManaValue, filter.ManaValueOp, err = parseComparison(ctx, "mana_value", 0, errInvalidManaValue, errInvalidManaValueOp); err != nil {
		return filter, err
	}
	if filter.Power, filter.PowerOp, err = parseComparison(ctx, "power", -1000000, errInvalidPower, errInvalidPowerOp); err != nil {
		return filter, err
	}
	if filter.Toughness, filter.ToughnessOp, err = parseComparison(ctx, "toughness", -1000000, errInvalidToughness, errInvalidToughnessOp); err != nil {
		return filter, err
	}

	if raw := ctx.Query("type"); raw != "" {
		if !scryfall.IsPrimaryType(raw) || raw == "Other" {
			return filter, errInvalidTypeFilter
		}
		filter.Type = raw
	}

	if raw := strings.TrimSpace(ctx.Query("subtype")); raw != "" {
		if len([]rune(raw)) > 50 {
			return filter, errInvalidSubtype
		}
		filter.Subtype = raw
	}

	if raw := ctx.Query("legal_in"); raw != "" {
		if !legalFormats[raw] {
			return filter, errInvalidLegalIn
		}
		filter.LegalIn = raw
	}

	if raw := ctx.Query("color_count"); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count < 0 || count > 5 {
			return filter, errInvalidColorCount
		}
		filter.ColorCount = &count
	}

	if raw := ctx.Query("foil"); raw != "" {
		foil, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errInvalidFoilFilter
		}
		filter.Foil = &foil
	}

	if raw := strings.TrimSpace(ctx.Query("storage_type")); raw != "" {
		if len([]rune(raw)) > 50 {
			return filter, errInvalidStorageType
		}
		filter.StorageType = raw
	}

	if raw := ctx.Query("open_to_deck"); raw != "" {
		deckID, valid := deck.ParseID(raw)
		if !valid {
			return filter, errInvalidOpenToDeck
		}
		filter.OpenToDeck = &deckID
	}

	return filter, nil
}

func parseComparison(ctx *gin.Context, key string, min float64, errValue, errOp error) (*float64, string, error) {
	raw := ctx.Query(key)
	if raw == "" {
		return nil, "", nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < min || value > 1000000 {
		return nil, "", errValue
	}
	op := ctx.Query(key + "_op")
	if op == "" {
		return &value, "eq", nil
	}
	if _, ok := comparisonOperators[op]; !ok {
		return nil, "", errOp
	}
	return &value, op, nil
}
