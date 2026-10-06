package deckinsights

import (
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

var typesByPrecedence = []string{
	"Land", "Creature", "Planeswalker", "Battle", "Instant", "Sorcery", "Artifact", "Enchantment",
}

func computeStats(cards []deck.DeckCard, scryfallByID map[string]scryfall.Card) DeckStats {
	stats := DeckStats{
		ManaCurve:      []ManaCurveBucket{},
		ColorBreakdown: make(map[string]int),
		TypeBreakdown:  make(map[string]int),
	}

	curve := make(map[int]int)
	var totalManaValue float64
	var nonlandCount int

	for _, c := range cards {
		stats.CardCount++

		sc, ok := scryfallByID[c.ScryfallID]
		if !ok {
			stats.TypeBreakdown["Unknown"]++
			continue
		}

		t := primaryType(sc.TypeLine)
		stats.TypeBreakdown[t]++

		if t == "Land" {
			stats.LandCount++
			continue
		}

		nonlandCount++
		totalManaValue += sc.CMC
		curve[int(sc.CMC)]++

		if len(sc.Colors) == 0 {
			stats.ColorBreakdown["C"]++
		} else {
			for _, color := range sc.Colors {
				stats.ColorBreakdown[color]++
			}
		}
	}

	stats.NonlandCount = nonlandCount
	if nonlandCount > 0 {
		stats.AverageManaValue = totalManaValue / float64(nonlandCount)
	}

	manaValues := make([]int, 0, len(curve))
	for mv := range curve {
		manaValues = append(manaValues, mv)
	}
	sort.Ints(manaValues)
	for _, mv := range manaValues {
		stats.ManaCurve = append(stats.ManaCurve, ManaCurveBucket{ManaValue: mv, Count: curve[mv]})
	}

	return stats
}

func primaryType(typeLine string) string {
	types := typeLine
	if idx := strings.Index(typeLine, "—"); idx >= 0 {
		types = typeLine[:idx]
	}
	for _, t := range typesByPrecedence {
		if strings.Contains(types, t) {
			return t
		}
	}
	return "Other"
}
