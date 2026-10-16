package deckinsights

import (
	"fmt"
	"regexp"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

const commanderFormat = "commander"

const deckSizeReasonPrefix = "deck size:"

type deckSizeRule struct {
	min   int
	exact bool
}

var deckSizeRules = map[string]deckSizeRule{
	"commander":       {min: 100, exact: true},
	"brawl":           {min: 100, exact: true},
	"duel":            {min: 100, exact: true},
	"paupercommander": {min: 100, exact: true},
	"predh":           {min: 100, exact: true},
	"gladiator":       {min: 100, exact: true},
	"oathbreaker":     {min: 60, exact: true},
	"standardbrawl":   {min: 60, exact: true},
	"standard":        {min: 60},
	"pioneer":         {min: 60},
	"modern":          {min: 60},
	"legacy":          {min: 60},
	"vintage":         {min: 60},
	"pauper":          {min: 60},
	"premodern":       {min: 60},
	"explorer":        {min: 60},
	"historic":        {min: 60},
	"timeless":        {min: 60},
	"alchemy":         {min: 60},
	"oldschool":       {min: 60},
	"penny":           {min: 60},
	"future":          {min: 60},
}

func checkDeckSize(report *LegalityReport, format string, cardCount int) {
	rule, ok := deckSizeRules[format]
	if !ok {
		return
	}
	var reason string
	switch {
	case rule.exact && cardCount != rule.min:
		reason = fmt.Sprintf("%s %d cards, %s requires exactly %d", deckSizeReasonPrefix, cardCount, format, rule.min)
	case !rule.exact && cardCount < rule.min:
		reason = fmt.Sprintf("%s %d cards, %s requires at least %d", deckSizeReasonPrefix, cardCount, format, rule.min)
	default:
		return
	}
	report.Issues = append([]LegalityIssue{{Reason: reason}}, report.Issues...)
	report.Legal = false
}

func checkLegality(d deck.Deck, cards []deck.DeckCard, scryfallByID map[string]scryfall.Card) (LegalityReport, error) {
	format := strings.ToLower(strings.TrimSpace(d.Format))
	report := LegalityReport{Format: d.Format, Legal: true}

	anyCardFound := false
	formatRecognized := false
	for _, c := range cards {
		sc, ok := scryfallByID[c.ScryfallID]
		if !ok {
			report.Issues = append(report.Issues, LegalityIssue{
				CardID: c.ID, CardName: c.Name, Board: c.Board,
				Reason: "could not verify legality: not found on scryfall",
			})
			report.Legal = false
			continue
		}
		anyCardFound = true

		status, known := sc.Legalities[format]
		if !known {
			continue
		}
		formatRecognized = true

		switch status {
		case "legal":
			// nothing to report
		case "not_legal":
			report.Issues = append(report.Issues, LegalityIssue{CardID: c.ID, CardName: c.Name, Board: c.Board, Reason: fmt.Sprintf("not legal in %s", d.Format)})
			report.Legal = false
		case "restricted":
			report.Issues = append(report.Issues, LegalityIssue{CardID: c.ID, CardName: c.Name, Board: c.Board, Reason: fmt.Sprintf("restricted in %s", d.Format)})
			report.Legal = false
		case "banned":
			report.Issues = append(report.Issues, LegalityIssue{CardID: c.ID, CardName: c.Name, Board: c.Board, Reason: fmt.Sprintf("banned in %s", d.Format)})
			report.Legal = false
		}
	}

	if !formatRecognized && anyCardFound {
		return LegalityReport{}, ErrUnknownFormat
	}

	main := mainBoard(cards)
	if format == commanderFormat {
		checkSingleton(&report, main, scryfallByID)
		checkColorIdentity(&report, d, cards, scryfallByID)
		checkPartnerPair(&report, d, cards, scryfallByID)
	}

	checkDeckSize(&report, format, len(main))

	return report, nil
}

func checkSingleton(report *LegalityReport, cards []deck.DeckCard, scryfallByID map[string]scryfall.Card) {
	counts := make(map[string]int)
	for _, c := range cards {
		if sc, ok := scryfallByID[c.ScryfallID]; ok && isBasicLand(sc.TypeLine) {
			continue
		}
		counts[c.Name]++
	}

	for name, count := range counts {
		if count > 1 {
			report.Issues = append(report.Issues, LegalityIssue{
				CardName: name,
				Reason:   fmt.Sprintf("singleton violation: %d copies in deck (commander allows only 1, except basic lands)", count),
			})
			report.Legal = false
		}
	}
}

func findCard(cards []deck.DeckCard, id *int) (deck.DeckCard, bool) {
	if id == nil {
		return deck.DeckCard{}, false
	}
	for _, c := range cards {
		if c.ID == *id {
			return c, true
		}
	}
	return deck.DeckCard{}, false
}

func checkColorIdentity(report *LegalityReport, d deck.Deck, cards []deck.DeckCard, scryfallByID map[string]scryfall.Card) {
	commanderCard, ok := findCard(cards, d.CommanderID)
	if !ok {
		return
	}
	commanderScryfall, ok := scryfallByID[commanderCard.ScryfallID]
	if !ok {
		return
	}

	identity := append([]string(nil), commanderScryfall.ColorIdentity...)
	if partnerCard, ok := findCard(cards, d.PartnerID); ok {
		identity = append(identity, scryfallByID[partnerCard.ScryfallID].ColorIdentity...)
	}
	allowed := make(map[string]bool, len(identity))
	for _, color := range identity {
		allowed[color] = true
	}
	commanderIdentity := scryfall.ColorCode(identity)

	for _, c := range cards {
		sc, ok := scryfallByID[c.ScryfallID]
		if !ok {
			continue
		}
		for _, color := range sc.ColorIdentity {
			if !allowed[color] {
				report.Issues = append(report.Issues, LegalityIssue{
					CardID: c.ID, CardName: c.Name, Board: c.Board,
					Reason: fmt.Sprintf(
						"outside commander's color identity (card: %s, commander: %s)",
						strings.Join(sc.ColorIdentity, ""), commanderIdentity,
					),
				})
				report.Legal = false
				break
			}
		}
	}
}

func checkPartnerPair(report *LegalityReport, d deck.Deck, cards []deck.DeckCard, scryfallByID map[string]scryfall.Card) {
	commanderCard, ok := findCard(cards, d.CommanderID)
	if !ok {
		return
	}
	partnerCard, ok := findCard(cards, d.PartnerID)
	if !ok {
		return
	}
	commander, ok := scryfallByID[commanderCard.ScryfallID]
	if !ok {
		return
	}
	partner, ok := scryfallByID[partnerCard.ScryfallID]
	if !ok {
		return
	}
	if canPair(commander, partner) {
		return
	}
	report.Issues = append(report.Issues, LegalityIssue{
		CardID: partnerCard.ID, CardName: partnerCard.Name, Board: partnerCard.Board,
		Reason: "these two commanders can't be paired",
	})
	report.Legal = false
}

type pairingAbilities struct {
	partner          bool
	partnerWith      string
	partnerQuality   string
	friendsForever   bool
	chooseBackground bool
	doctorsCompanion bool
	background       bool
	timeLordDoctor   bool
	frontFaceName    string
}

var reminderText = regexp.MustCompile(`\s*\([^)]*\)`)

func frontFace(text string, separator string) string {
	front, _, _ := strings.Cut(text, separator)
	return strings.TrimSpace(front)
}

func abilitiesOf(c scryfall.Card) pairingAbilities {
	abilities := pairingAbilities{frontFaceName: frontFace(c.Name, "//")}

	typeLine := frontFace(c.TypeLine, "//")
	types, subtypes, _ := strings.Cut(typeLine, "—")
	legendary := strings.Contains(types, "Legendary")
	abilities.background = legendary && strings.Contains(subtypes, "Background")
	abilities.timeLordDoctor = legendary && strings.Contains(types, "Creature") &&
		strings.Contains(subtypes, "Time Lord") && strings.Contains(subtypes, "Doctor")

	oracle := strings.ReplaceAll(frontFace(c.OracleText, "\n//\n"), "’", "'")
	for _, line := range strings.Split(oracle, "\n") {
		line = strings.TrimSpace(reminderText.ReplaceAllString(line, ""))
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "partner with ") {
			abilities.partnerWith = strings.TrimSpace(line[len("partner with "):])
			continue
		}
		for _, keyword := range strings.Split(lower, ",") {
			keyword = strings.TrimSpace(keyword)
			switch {
			case keyword == "partner":
				abilities.partner = true
			case strings.HasPrefix(keyword, "partner—"):
				abilities.partnerQuality = strings.TrimSpace(strings.TrimPrefix(keyword, "partner—"))
			case keyword == "friends forever":
				abilities.friendsForever = true
			case keyword == "choose a background":
				abilities.chooseBackground = true
			case keyword == "doctor's companion":
				abilities.doctorsCompanion = true
			}
		}
	}
	return abilities
}

func canPair(first, second scryfall.Card) bool {
	a, b := abilitiesOf(first), abilitiesOf(second)
	switch {
	case a.partner && b.partner:
		return true
	case a.partnerWith != "" && strings.EqualFold(a.partnerWith, b.frontFaceName) && strings.EqualFold(b.partnerWith, a.frontFaceName):
		return true
	case a.partnerQuality != "" && a.partnerQuality == b.partnerQuality:
		return true
	case a.friendsForever && b.friendsForever:
		return true
	case a.chooseBackground && b.background, b.chooseBackground && a.background:
		return true
	case a.doctorsCompanion && b.timeLordDoctor, b.doctorsCompanion && a.timeLordDoctor:
		return true
	}
	return false
}

func mainBoard(cards []deck.DeckCard) []deck.DeckCard {
	main := make([]deck.DeckCard, 0, len(cards))
	for _, c := range cards {
		if deck.InMainBoard(c.Board) {
			main = append(main, c)
		}
	}
	return main
}

func inBoard(cards []deck.DeckCard, board string) []deck.DeckCard {
	var matching []deck.DeckCard
	for _, c := range cards {
		if c.Board == board {
			matching = append(matching, c)
		}
	}
	return matching
}

func isBasicLand(typeLine string) bool {
	return strings.Contains(typeLine, "Basic Land")
}
