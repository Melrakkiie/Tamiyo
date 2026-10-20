package card

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

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
	errInvalidScryfallID   = errors.New("scryfall_id must be a Scryfall card id")
)

type numericFilter struct {
	key, field      string
	min             float64
	errValue, errOp error
}

var numericFilters = []numericFilter{
	{"mana_value", fieldManaValue, 0, errInvalidManaValue, errInvalidManaValueOp},
	{"power", fieldPower, -1000000, errInvalidPower, errInvalidPowerOp},
	{"toughness", fieldToughness, -1000000, errInvalidToughness, errInvalidToughnessOp},
}

var scryfallIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var colorModeOperators = map[string]string{deck.ColorModeExact: "=", deck.ColorModeInclude: ">=", deck.ColorModeWithin: "<="}

func parseAdvancedFilters(query url.Values) (CardFilter, error) {
	var filter CardFilter
	var criteria []*SearchNode

	if query.Has("colors") {
		upper := strings.ToUpper(query.Get("colors"))
		if strings.Trim(upper, "WUBRG") != "" {
			return filter, errInvalidFilterColors
		}
		mode := query.Get("color_mode")
		if mode == "" {
			mode = deck.ColorModeExact
		}
		op, known := colorModeOperators[mode]
		if !known {
			return filter, errInvalidColorMode
		}
		criteria = append(criteria, term(fieldColor, op, scryfall.ColorCode(strings.Split(upper, "")), 0))
	}

	for _, stat := range numericFilters {
		criterion, err := parseComparison(query, stat)
		if err != nil {
			return filter, err
		}
		if criterion != nil {
			criteria = append(criteria, criterion)
		}
	}

	if raw := query.Get("type"); raw != "" {
		if !scryfall.IsPrimaryType(raw) || raw == "Other" {
			return filter, errInvalidTypeFilter
		}
		criteria = append(criteria, term(fieldType, "=", raw, 0))
	}

	if raw := strings.TrimSpace(query.Get("subtype")); raw != "" {
		if len([]rune(raw)) > 50 {
			return filter, errInvalidSubtype
		}
		criteria = append(criteria, term(fieldSubtype, "=", raw, 0))
	}

	if raw := query.Get("legal_in"); raw != "" {
		if !legalFormats[raw] {
			return filter, errInvalidLegalIn
		}
		criteria = append(criteria, term(fieldFormat, "=", raw, 0))
	}

	if raw := query.Get("color_count"); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil || count < 0 || count > 5 {
			return filter, errInvalidColorCount
		}
		criteria = append(criteria, term(fieldColorCount, "=", "", float64(count)))
	}

	if raw := query.Get("foil"); raw != "" {
		foil, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errInvalidFoilFilter
		}
		state := copyStateNonFoil
		if foil {
			state = copyStateFoil
		}
		criteria = append(criteria, term(fieldIs, "=", state, 0))
	}

	if raw := strings.TrimSpace(query.Get("storage_type")); raw != "" {
		if len([]rune(raw)) > 50 {
			return filter, errInvalidStorageType
		}
		criteria = append(criteria, term(fieldStorageType, "=", raw, 0))
	}

	if raw := query.Get("open_to_deck"); raw != "" {
		deckID, valid := deck.ParseID(raw)
		if !valid {
			return filter, errInvalidOpenToDeck
		}
		filter.OpenToDeck = &deckID
	}

	search, err := parseSearchQuery(query.Get("q"))
	if err != nil {
		return filter, err
	}
	filter.Query = allOf(append(criteria, search)...)

	if raw := query.Get("scryfall_id"); raw != "" {
		scryfallID := strings.ToLower(raw)
		if !scryfallIDPattern.MatchString(scryfallID) {
			return filter, errInvalidScryfallID
		}
		filter.ScryfallID = scryfallID
	}

	return filter, nil
}

func parseComparison(query url.Values, f numericFilter) (*SearchNode, error) {
	raw := query.Get(f.key)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < f.min || value > 1000000 {
		return nil, f.errValue
	}
	name := query.Get(f.key + "_op")
	if name == "" {
		name = "eq"
	}
	op, ok := comparisonOperators[name]
	if !ok {
		return nil, f.errOp
	}
	return term(f.field, op, "", value), nil
}
