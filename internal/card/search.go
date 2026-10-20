package card

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"Melrakkiie/Tamiyo/internal/scryfall"
)

var errInvalidQuery = errors.New("q")

const (
	maxQueryLength = 500
	maxQueryTerms  = 50
)

type SearchNodeKind int

const (
	SearchAnd SearchNodeKind = iota
	SearchOr
	SearchNot
	SearchTerm
)

type SearchNode struct {
	Kind     SearchNodeKind
	Children []*SearchNode
	Term     SearchCriterion
}

type SearchCriterion struct {
	Field  string
	Op     string
	Text   string
	Number float64
}

const (
	fieldName          = "name"
	fieldExactName     = "exact"
	fieldType          = "type"
	fieldOracle        = "oracle"
	fieldColor         = "color"
	fieldColorCount    = "color_count"
	fieldIdentity      = "identity"
	fieldIdentityCount = "identity_count"
	fieldManaValue     = "mana_value"
	fieldManaParity    = "mana_parity"
	fieldPower         = "power"
	fieldToughness     = "toughness"
	fieldManaCost      = "mana_cost"
	fieldRarity        = "rarity"
	fieldSet           = "set"
	fieldNumber        = "number"
	fieldFormat        = "format"
	fieldKeyword       = "keyword"
	fieldIs            = "is"
	fieldNot           = "not"
	fieldUSD           = "usd"
	fieldEUR           = "eur"

	// Internal-only criteria: produced by the advanced parameters, never by a
	// q= keyword (they have no entry in searchKeys).
	fieldSubtype     = "subtype"
	fieldStorageType = "storage_type"
)

var searchKeys = map[string]string{
	"name": fieldName,
	"t":    fieldType, "type": fieldType,
	"o": fieldOracle, "oracle": fieldOracle,
	"c": fieldColor, "color": fieldColor,
	"id": fieldIdentity, "identity": fieldIdentity, "ci": fieldIdentity,
	"mv": fieldManaValue, "cmc": fieldManaValue, "manavalue": fieldManaValue,
	"pow": fieldPower, "power": fieldPower,
	"tou": fieldToughness, "toughness": fieldToughness,
	"m": fieldManaCost, "mana": fieldManaCost,
	"r": fieldRarity, "rarity": fieldRarity,
	"s": fieldSet, "set": fieldSet, "e": fieldSet, "edition": fieldSet,
	"cn": fieldNumber, "number": fieldNumber,
	"f": fieldFormat, "format": fieldFormat, "legal": fieldFormat,
	"kw": fieldKeyword, "keyword": fieldKeyword,
	"is":  fieldIs,
	"not": fieldNot,
	"usd": fieldUSD,
	"eur": fieldEUR,
}

var colorNames = map[string]string{
	"white": "W", "blue": "U", "black": "B", "red": "R", "green": "G",
	"azorius": "WU", "dimir": "UB", "rakdos": "BR", "gruul": "RG", "selesnya": "GW",
	"orzhov": "WB", "izzet": "UR", "golgari": "BG", "boros": "RW", "simic": "GU",
	"bant": "GWU", "esper": "WUB", "grixis": "UBR", "jund": "BRG", "naya": "RGW",
	"abzan": "WBG", "jeskai": "URW", "sultai": "BGU", "mardu": "RWB", "temur": "GUR",
}

var rarityRanks = map[string]int{
	"c": 1, "common": 1,
	"u": 2, "uncommon": 2,
	"r": 3, "rare": 3,
	"m": 4, "mythic": 4,
}

var ownedCopyStates = map[string]bool{"foil": true, "nonfoil": true, "proxy": true}

type tokenKind int

const (
	tokenTerm tokenKind = iota
	tokenExactName
	tokenOr
	tokenNot
	tokenOpen
	tokenClose
)

type queryToken struct {
	kind  tokenKind
	key   string
	op    string
	value string
}

func parseSearchQuery(raw string) (*SearchNode, error) {
	runes := []rune(strings.TrimSpace(raw))
	if len(runes) == 0 {
		return nil, nil
	}
	if len(runes) > maxQueryLength {
		return nil, fmt.Errorf("%w: must have at most %d characters", errInvalidQuery, maxQueryLength)
	}
	tokens, err := lexQuery(runes)
	if err != nil {
		return nil, err
	}
	p := &queryParser{tokens: tokens}
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("%w: unexpected \")\"", errInvalidQuery)
	}
	return node, nil
}

func lexQuery(runes []rune) ([]queryToken, error) {
	var tokens []queryToken
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '(':
			tokens = append(tokens, queryToken{kind: tokenOpen})
			i++
		case r == ')':
			tokens = append(tokens, queryToken{kind: tokenClose})
			i++
		case r == '-' && startsTerm(runes, i+1):
			tokens = append(tokens, queryToken{kind: tokenNot})
			i++
		case r == '!' && startsTerm(runes, i+1):
			value, next, err := readValue(runes, i+1)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, queryToken{kind: tokenExactName, value: value})
			i = next
		default:
			token, next, err := readTerm(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			i = next
		}
	}
	return tokens, nil
}

func startsTerm(runes []rune, i int) bool {
	return i < len(runes) && !unicode.IsSpace(runes[i]) && runes[i] != ')'
}

func readTerm(runes []rune, start int) (queryToken, int, error) {
	if runes[start] == '"' {
		value, next, err := readValue(runes, start)
		return queryToken{kind: tokenTerm, value: value}, next, err
	}

	end := start
	for end < len(runes) && unicode.IsLetter(runes[end]) {
		end++
	}
	if op := operatorAt(runes, end); end > start && op != "" {
		key := strings.ToLower(string(runes[start:end]))
		value, next, err := readValue(runes, end+len(op))
		if err != nil {
			return queryToken{}, 0, err
		}
		if value == "" {
			return queryToken{}, 0, fmt.Errorf("%w: %q needs a value", errInvalidQuery, key+op)
		}
		return queryToken{kind: tokenTerm, key: key, op: op, value: value}, next, nil
	}

	for end < len(runes) && !unicode.IsSpace(runes[end]) && runes[end] != '(' && runes[end] != ')' {
		end++
	}
	word := string(runes[start:end])
	if strings.EqualFold(word, "or") {
		return queryToken{kind: tokenOr}, end, nil
	}
	return queryToken{kind: tokenTerm, value: word}, end, nil
}

func operatorAt(runes []rune, i int) string {
	if i >= len(runes) {
		return ""
	}
	next := rune(0)
	if i+1 < len(runes) {
		next = runes[i+1]
	}
	switch runes[i] {
	case ':', '=':
		return string(runes[i])
	case '!':
		if next == '=' {
			return "!="
		}
	case '<', '>':
		if next == '=' {
			return string(runes[i]) + "="
		}
		return string(runes[i])
	}
	return ""
}

func readValue(runes []rune, start int) (string, int, error) {
	if start < len(runes) && runes[start] == '"' {
		for end := start + 1; end < len(runes); end++ {
			if runes[end] == '"' {
				return string(runes[start+1 : end]), end + 1, nil
			}
		}
		return "", 0, fmt.Errorf("%w: missing closing quote", errInvalidQuery)
	}
	end := start
	for end < len(runes) && !unicode.IsSpace(runes[end]) && runes[end] != ')' {
		end++
	}
	return string(runes[start:end]), end, nil
}

type queryParser struct {
	tokens []queryToken
	pos    int
	depth  int
	terms  int
}

func (p *queryParser) at(kind tokenKind) bool {
	return p.pos < len(p.tokens) && p.tokens[p.pos].kind == kind
}

func (p *queryParser) parseOr() (*SearchNode, error) {
	first, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	children := []*SearchNode{first}
	for p.at(tokenOr) {
		p.pos++
		next, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		children = append(children, next)
	}
	if len(children) == 1 {
		return first, nil
	}
	return &SearchNode{Kind: SearchOr, Children: children}, nil
}

func (p *queryParser) parseAnd() (*SearchNode, error) {
	var children []*SearchNode
	for p.pos < len(p.tokens) && !p.at(tokenOr) && !p.at(tokenClose) {
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	switch {
	case len(children) == 1:
		return children[0], nil
	case len(children) > 1:
		return &SearchNode{Kind: SearchAnd, Children: children}, nil
	case p.at(tokenOr) || (p.pos > 0 && p.tokens[p.pos-1].kind == tokenOr):
		return nil, fmt.Errorf("%w: \"or\" must be between two terms", errInvalidQuery)
	case p.at(tokenClose) && p.depth > 0:
		return nil, fmt.Errorf("%w: empty parentheses", errInvalidQuery)
	default:
		return nil, fmt.Errorf("%w: unexpected \")\"", errInvalidQuery)
	}
}

func (p *queryParser) parseUnary() (*SearchNode, error) {
	token := p.tokens[p.pos]
	p.pos++
	switch token.kind {
	case tokenNot:
		if p.pos >= len(p.tokens) || p.at(tokenOr) || p.at(tokenClose) {
			return nil, fmt.Errorf("%w: \"-\" must precede a term", errInvalidQuery)
		}
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return negate(child), nil
	case tokenOpen:
		p.depth++
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.at(tokenClose) {
			return nil, fmt.Errorf("%w: missing closing parenthesis", errInvalidQuery)
		}
		p.pos++
		p.depth--
		return node, nil
	default:
		p.terms++
		if p.terms > maxQueryTerms {
			return nil, fmt.Errorf("%w: must have at most %d terms", errInvalidQuery, maxQueryTerms)
		}
		return buildTerm(token)
	}
}

func negate(node *SearchNode) *SearchNode {
	return &SearchNode{Kind: SearchNot, Children: []*SearchNode{node}}
}

// allOf joins the non-nil nodes under one AND: nil when there is none, the
// node itself when there is one.
func allOf(nodes ...*SearchNode) *SearchNode {
	children := make([]*SearchNode, 0, len(nodes))
	for _, node := range nodes {
		if node != nil {
			children = append(children, node)
		}
	}
	switch len(children) {
	case 0:
		return nil
	case 1:
		return children[0]
	default:
		return &SearchNode{Kind: SearchAnd, Children: children}
	}
}

func term(field, op, text string, number float64) *SearchNode {
	return &SearchNode{Kind: SearchTerm, Term: SearchCriterion{Field: field, Op: op, Text: text, Number: number}}
}

func buildTerm(token queryToken) (*SearchNode, error) {
	if token.kind == tokenExactName {
		if token.value == "" {
			return nil, fmt.Errorf("%w: \"!\" needs a card name", errInvalidQuery)
		}
		return term(fieldExactName, "=", token.value, 0), nil
	}
	if token.key == "" {
		return term(fieldName, "=", token.value, 0), nil
	}

	field, known := searchKeys[token.key]
	if !known {
		return nil, fmt.Errorf("%w: unknown keyword %q", errInvalidQuery, token.key)
	}
	unsupported := fmt.Errorf("%w: %s does not support %q", errInvalidQuery, token.key, token.op)
	if token.op == "!=" {
		if field == fieldIs || field == fieldNot {
			return nil, unsupported
		}
		node, err := buildCriterion(field, "=", token.value, unsupported)
		if err != nil {
			return nil, err
		}
		return negate(node), nil
	}
	return buildCriterion(field, token.op, token.value, unsupported)
}

func buildCriterion(field, op, value string, unsupported error) (*SearchNode, error) {
	equality := op == ":" || op == "="
	switch field {
	case fieldName, fieldType, fieldOracle, fieldKeyword:
		if !equality {
			return nil, unsupported
		}
		return term(field, "=", value, 0), nil
	case fieldColor, fieldIdentity:
		return colorCriterion(field, op, value, unsupported)
	case fieldManaValue:
		if parity := strings.ToLower(value); parity == "even" || parity == "odd" {
			if !equality {
				return nil, unsupported
			}
			return term(fieldManaParity, "=", parity, 0), nil
		}
		return numberCriterion(field, op, value)
	case fieldPower, fieldToughness, fieldUSD, fieldEUR:
		return numberCriterion(field, op, value)
	case fieldManaCost:
		return manaCostCriterion(op, value, unsupported)
	case fieldRarity:
		rank, ok := rarityRanks[strings.ToLower(value)]
		if !ok {
			return nil, fmt.Errorf("%w: %q is not a rarity (common, uncommon, rare, mythic)", errInvalidQuery, value)
		}
		return term(field, comparison(op), "", float64(rank)), nil
	case fieldSet:
		if !equality {
			return nil, unsupported
		}
		return term(field, "=", strings.ToLower(value), 0), nil
	case fieldNumber:
		if equality {
			return term(field, "=", value, 0), nil
		}
		return numberCriterion(field, op, value)
	case fieldFormat:
		format := strings.ToLower(value)
		if !equality {
			return nil, unsupported
		}
		if !legalFormats[format] {
			return nil, fmt.Errorf("%w: %q is not a format", errInvalidQuery, value)
		}
		return term(field, "=", format, 0), nil
	default:
		state := strings.ToLower(value)
		if op != ":" {
			return nil, unsupported
		}
		if !ownedCopyStates[state] {
			return nil, fmt.Errorf("%w: %q is not supported by %s: (foil, nonfoil, proxy)", errInvalidQuery, value, field)
		}
		node := term(fieldIs, "=", state, 0)
		if field == fieldNot {
			return negate(node), nil
		}
		return node, nil
	}
}

func comparison(op string) string {
	if op == ":" {
		return "="
	}
	return op
}

func numberCriterion(field, op, value string) (*SearchNode, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.Abs(number) > 1000000 {
		return nil, fmt.Errorf("%w: %q is not a number", errInvalidQuery, value)
	}
	return term(field, comparison(op), "", number), nil
}

func colorCriterion(field, op, value string, unsupported error) (*SearchNode, error) {
	countField := fieldColorCount
	if field == fieldIdentity {
		countField = fieldIdentityCount
	}
	lower := strings.ToLower(value)

	if count, err := strconv.Atoi(lower); err == nil {
		if count < 0 || count > 5 {
			return nil, fmt.Errorf("%w: %q is not a color", errInvalidQuery, value)
		}
		return term(countField, comparison(op), "", float64(count)), nil
	}

	if lower == "m" || lower == "multicolor" {
		if op != ":" && op != "=" {
			return nil, unsupported
		}
		return term(countField, ">=", "", 2), nil
	}

	var code string
	switch {
	case lower == "c" || lower == "colorless":
		code = ""
	case colorNames[lower] != "":
		code = scryfall.ColorCode(strings.Split(colorNames[lower], ""))
	case strings.Trim(lower, "wubrg") == "":
		code = scryfall.ColorCode(strings.Split(strings.ToUpper(lower), ""))
	default:
		return nil, fmt.Errorf("%w: %q is not a color", errInvalidQuery, value)
	}

	if op == ":" {
		switch {
		case field == fieldIdentity:
			op = "<="
		case code == "":
			op = "="
		default:
			op = ">="
		}
	}
	return term(field, op, code, 0), nil
}

func manaCostCriterion(op, value string, unsupported error) (*SearchNode, error) {
	switch op {
	case ":":
		op = ">="
	case "=", ">=", ">":
	default:
		return nil, unsupported
	}
	cost, ok := normalizeManaCost(value)
	if !ok {
		return nil, fmt.Errorf("%w: %q is not a mana cost", errInvalidQuery, value)
	}
	return term(fieldManaCost, op, cost, 0), nil
}

func normalizeManaCost(value string) (string, bool) {
	runes := []rune(strings.ToUpper(value))
	var b strings.Builder
	for i := 0; i < len(runes); {
		switch r := runes[i]; {
		case r == '{':
			end := i + 1
			for end < len(runes) && runes[end] != '}' {
				end++
			}
			if end >= len(runes) || end == i+1 {
				return "", false
			}
			b.WriteString(string(runes[i : end+1]))
			i = end + 1
		case unicode.IsDigit(r):
			end := i
			for end < len(runes) && unicode.IsDigit(runes[end]) {
				end++
			}
			b.WriteString("{" + string(runes[i:end]) + "}")
			i = end
		case strings.ContainsRune("WUBRGCXYZS", r):
			b.WriteString("{" + string(r) + "}")
			i++
		default:
			return "", false
		}
	}
	return b.String(), b.Len() > 0
}
