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

var scryfallIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// colorModeOperators maps each advanced color mode to the operator of the
// search syntax's color criterion: exact is c=, include c>= and within c<=.
var colorModeOperators = map[string]string{"exact": "=", "include": ">=", "within": "<="}

// parseAdvancedFilters validates the advanced search parameters and translates
// them into search criteria, joined with the parsed q= search under one AND in
// the filter's Query, so the search compiler alone turns them into SQL.
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
			mode = "exact"
		}
		op, known := colorModeOperators[mode]
		if !known {
			return filter, errInvalidColorMode
		}
		criteria = append(criteria, term(fieldColor, op, scryfall.ColorCode(strings.Split(upper, "")), 0))
	}

	for _, stat := range []struct {
		key, field      string
		min             float64
		errValue, errOp error
	}{
		{"mana_value", fieldManaValue, 0, errInvalidManaValue, errInvalidManaValueOp},
		{"power", fieldPower, -1000000, errInvalidPower, errInvalidPowerOp},
		{"toughness", fieldToughness, -1000000, errInvalidToughness, errInvalidToughnessOp},
	} {
		criterion, err := parseComparison(query, stat.key, stat.field, stat.min, stat.errValue, stat.errOp)
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
		filter.Type = raw
	}

	if raw := strings.TrimSpace(query.Get("subtype")); raw != "" {
		if len([]rune(raw)) > 50 {
			return filter, errInvalidSubtype
		}
		filter.Subtype = raw
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
		state := "nonfoil"
		if foil {
			state = "foil"
		}
		criteria = append(criteria, term(fieldIs, "=", state, 0))
	}

	if raw := strings.TrimSpace(query.Get("storage_type")); raw != "" {
		if len([]rune(raw)) > 50 {
			return filter, errInvalidStorageType
		}
		filter.StorageType = raw
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

// parseComparison translates a numeric parameter and its _op companion (eq by
// default) into the search criterion on field with the same operator.
func parseComparison(query url.Values, key, field string, min float64, errValue, errOp error) (*SearchNode, error) {
	raw := query.Get(key)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < min || value > 1000000 {
		return nil, errValue
	}
	name := query.Get(key + "_op")
	if name == "" {
		name = "eq"
	}
	op, ok := comparisonOperators[name]
	if !ok {
		return nil, errOp
	}
	return term(field, op, "", value), nil
}
