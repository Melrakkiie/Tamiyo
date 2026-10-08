package deckinsights

import (
	"fmt"
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

func checkColorIdentity(report *LegalityReport, d deck.Deck, cards []deck.DeckCard, scryfallByID map[string]scryfall.Card) {
	if d.CommanderID == nil {
		return
	}

	var commanderCard *deck.DeckCard
	for i := range cards {
		if cards[i].ID == *d.CommanderID {
			commanderCard = &cards[i]
			break
		}
	}
	if commanderCard == nil {
		return
	}
	commanderScryfall, ok := scryfallByID[commanderCard.ScryfallID]
	if !ok {
		return
	}

	allowed := make(map[string]bool, len(commanderScryfall.ColorIdentity))
	for _, color := range commanderScryfall.ColorIdentity {
		allowed[color] = true
	}

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
						strings.Join(sc.ColorIdentity, ""), strings.Join(commanderScryfall.ColorIdentity, ""),
					),
				})
				report.Legal = false
				break
			}
		}
	}
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

func isBasicLand(typeLine string) bool {
	return strings.Contains(typeLine, "Basic Land")
}
