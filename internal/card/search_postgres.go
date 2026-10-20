package card

import "strings"

var searchComparisons = map[string]string{"=": "=", "<": "<", "<=": "<=", ">": ">", ">=": ">="}

const rarityRankSQL = `CASE p.rarity WHEN 'common' THEN 1 WHEN 'uncommon' THEN 2 WHEN 'rare' THEN 3 WHEN 'mythic' THEN 4 END`

func searchCondition(node *SearchNode, bind func(any) string) string {
	switch node.Kind {
	case SearchAnd, SearchOr:
		separator := " AND "
		if node.Kind == SearchOr {
			separator = " OR "
		}
		parts := make([]string, len(node.Children))
		for i, child := range node.Children {
			parts[i] = searchCondition(child, bind)
		}
		return "(" + strings.Join(parts, separator) + ")"
	case SearchNot:
		return "NOT COALESCE(" + searchCondition(node.Children[0], bind) + ", false)"
	default:
		return criterionCondition(node.Term, bind)
	}
}

func criterionCondition(c SearchCriterion, bind func(any) string) string {
	op := searchComparisons[c.Op]
	switch c.Field {
	case fieldName:
		return "cards.name ILIKE " + bind(containsPattern(c.Text))
	case fieldExactName:
		return "lower(cards.name) = lower(" + bind(c.Text) + ")"
	case fieldType:
		pattern := bind(containsPattern(c.Text))
		return `(` + printingExists(`p.type_line <> '' AND p.type_line ILIKE `+pattern) +
			` OR (cards.card_type ILIKE ` + pattern + ` AND NOT ` + printingExists(`p.type_line <> ''`) + `))`
	case fieldOracle:
		pattern := bind(containsPattern(c.Text))
		if strings.Contains(c.Text, "~") {
			pattern = "replace(" + pattern + ", '~', cards.name)"
		}
		return printingExists("p.oracle_text ILIKE " + pattern)
	case fieldColor:
		return colorSetCondition("cards.colors", op, bind(c.Text))
	case fieldIdentity:
		return colorSetCondition("cards.color_identity", op, bind(c.Text))
	case fieldColorCount:
		return "(cards.colors IS NOT NULL AND length(cards.colors) " + op + " " + bind(int(c.Number)) + ")"
	case fieldIdentityCount:
		return "(cards.color_identity IS NOT NULL AND length(cards.color_identity) " + op + " " + bind(int(c.Number)) + ")"
	case fieldManaValue:
		return "cards.mana_value " + op + " " + bind(c.Number)
	case fieldManaParity:
		remainder := "0"
		if c.Text == "odd" {
			remainder = "1"
		}
		return "(cards.mana_value = floor(cards.mana_value) AND mod(cards.mana_value::numeric, 2) = " + remainder + ")"
	case fieldPower:
		return printingStatCondition("powers", op, bind(c.Number))
	case fieldToughness:
		return printingStatCondition("toughnesses", op, bind(c.Number))
	case fieldManaCost:
		return printingExists(manaCostCondition(c, bind))
	case fieldRarity:
		return printingExists(rarityRankSQL + " " + op + " " + bind(int(c.Number)))
	case fieldSet:
		return "lower(cards.set_code) = " + bind(c.Text)
	case fieldNumber:
		if c.Text != "" {
			return "cards.collector_number = " + bind(c.Text)
		}
		return "(CASE WHEN cards.collector_number ~ '^[0-9]+$' THEN cards.collector_number::numeric END) " + op + " " + bind(c.Number)
	case fieldFormat:
		return printingExists(bind(c.Text) + " = ANY(p.legal_formats)")
	case fieldSubtype:
		// The text after the type line's first dash; % and _ keep their
		// pattern meaning, as the advanced subtype parameter always had.
		return printingExists("split_part(p.type_line, '—', 2) ILIKE " + bind("%"+c.Text+"%"))
	case fieldStorageType:
		// The outer user_id = $1 scoping is what keeps this safe; matching the
		// owner here only keeps the subquery from reading across users.
		return "cards.storage_id IN (SELECT s.id FROM tamiyo.storage s WHERE s.user_id = cards.user_id AND lower(trim(s.type)) = lower(trim(" + bind(c.Text) + ")))"
	case fieldKeyword:
		return `EXISTS (SELECT 1 FROM tamiyo.printings p, unnest(p.keywords) k WHERE p.scryfall_id = cards.scryfall_id AND lower(k) = lower(` + bind(c.Text) + `))`
	case fieldIs:
		switch c.Text {
		case copyStateFoil:
			return "cards.foil"
		case copyStateNonFoil:
			return "NOT cards.foil"
		default:
			return "cards.proxy"
		}
	default:
		return "tamiyo.card_price(cards.scryfall_id, cards.foil, " + bind(c.Field) + ") " + op + " " + bind(c.Number)
	}
}

func printingExists(condition string) string {
	return "EXISTS (SELECT 1 FROM tamiyo.printings p WHERE p.scryfall_id = cards.scryfall_id AND " + condition + ")"
}

func printingStatCondition(column, op, value string) string {
	return `EXISTS (
			SELECT 1 FROM tamiyo.printings p, unnest(p.` + column + `) v
			WHERE p.scryfall_id = cards.scryfall_id
			  AND (CASE WHEN v ~ '^-?[0-9]*\.?[0-9]+$' THEN v::numeric END) ` + op + ` ` + value + `
		)`
}

func containsPattern(text string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text) + "%"
}

func colorSetCondition(column, op, value string) string {
	var condition string
	switch op {
	case ">=":
		condition = "translate(" + value + ", " + column + ", '') = ''"
	case "<=":
		condition = "translate(" + column + ", " + value + ", '') = ''"
	case ">":
		condition = "translate(" + value + ", " + column + ", '') = '' AND " + column + " <> " + value
	case "<":
		condition = "translate(" + column + ", " + value + ", '') = '' AND " + column + " <> " + value
	default:
		condition = column + " = " + value
	}
	return "(" + column + " IS NOT NULL AND " + condition + ")"
}

func manaCostCondition(c SearchCriterion, bind func(any) string) string {
	if c.Op == "=" {
		return "p.mana_cost = " + bind(c.Text)
	}
	symbols := strings.SplitAfter(c.Text, "}")
	symbols = symbols[:len(symbols)-1]
	counts := make(map[string]int, len(symbols))
	order := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		if counts[symbol] == 0 {
			order = append(order, symbol)
		}
		counts[symbol]++
	}
	conditions := make([]string, 0, len(order)+1)
	for _, symbol := range order {
		conditions = append(conditions, "length(p.mana_cost) - length(replace(p.mana_cost, "+bind(symbol)+", '')) >= "+bind(counts[symbol]*len(symbol)))
	}
	if c.Op == ">" {
		conditions = append(conditions, "p.mana_cost <> "+bind(c.Text))
	}
	return strings.Join(conditions, " AND ")
}
