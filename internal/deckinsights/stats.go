package deckinsights

import (
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

func computeStats(cards []deck.DeckCard, scryfallByID map[string]scryfall.Card) DeckStats {
	stats := DeckStats{
		ManaCurve:      []ManaCurveBucket{},
		ColorBreakdown: make(map[string]int),
		TypeBreakdown:  make(map[string]int),
	}

	curve := make(map[int]*ManaCurveBucket)
	cardIndex := make(map[int]map[string]int)
	var totalManaValue float64
	var nonlandCount int

	for _, c := range cards {
		stats.CardCount++

		sc, ok := scryfallByID[c.ScryfallID]
		if !ok {
			stats.TypeBreakdown["Unknown"]++
			continue
		}

		t := scryfall.PrimaryType(sc.TypeLine)
		stats.TypeBreakdown[t]++

		if t == "Land" {
			stats.LandCount++
			continue
		}

		nonlandCount++
		totalManaValue += sc.CMC
		addToCurve(curve, cardIndex, int(sc.CMC), c, t, isPermanent(sc.TypeLine))

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
		bucket := curve[mv]
		sort.SliceStable(bucket.Cards, func(i, j int) bool {
			return strings.ToLower(bucket.Cards[i].Name) < strings.ToLower(bucket.Cards[j].Name)
		})
		stats.ManaCurve = append(stats.ManaCurve, *bucket)
	}

	return stats
}

func isPermanent(typeLine string) bool {
	front, _, _ := strings.Cut(typeLine, "//")
	return !strings.Contains(front, "Instant") && !strings.Contains(front, "Sorcery")
}

func addToCurve(curve map[int]*ManaCurveBucket, cardIndex map[int]map[string]int, mv int, c deck.DeckCard, cardType string, permanent bool) {
	bucket, ok := curve[mv]
	if !ok {
		bucket = &ManaCurveBucket{ManaValue: mv, Cards: []ManaCurveCard{}}
		curve[mv] = bucket
		cardIndex[mv] = make(map[string]int)
	}
	bucket.Count++
	if permanent {
		bucket.Permanents++
	} else {
		bucket.NonPermanents++
	}
	if i, ok := cardIndex[mv][c.Name]; ok {
		bucket.Cards[i].Quantity++
		return
	}
	cardIndex[mv][c.Name] = len(bucket.Cards)
	bucket.Cards = append(bucket.Cards, ManaCurveCard{Name: c.Name, ScryfallID: c.ScryfallID, Quantity: 1, Type: cardType})
}
