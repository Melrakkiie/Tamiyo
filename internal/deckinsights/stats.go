package deckinsights

import (
	"math"
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

func computeStats(entries []deck.DeckEntry, printings map[string]Printing) DeckStats {
	stats := DeckStats{
		ManaCurve:      []ManaCurveBucket{},
		BackFaceLands:  []BackFaceCard{},
		ColorBreakdown: make(map[string]int),
		TypeBreakdown:  make(map[string]int),
	}

	curve := make(map[int]*ManaCurveBucket)
	cardIndex := make(map[int]map[string]int)
	backFaceIndex := make(map[string]int)
	gameChangers := make(map[string]bool)
	var totalManaValue float64
	var nonlandCount int

	for _, e := range entries {
		quantity := e.Quantity()
		stats.CardCount += quantity

		p, ok := printings[e.ScryfallID]
		if !ok {
			stats.TypeBreakdown["Unknown"] += quantity
			continue
		}
		if p.GameChanger {
			gameChangers[deck.CardNameKey(e.Name)] = true
		}

		t := scryfall.PrimaryType(p.TypeLine)
		stats.TypeBreakdown[t] += quantity

		if t == "Land" {
			stats.LandCount += quantity
			continue
		}

		nonlandCount += quantity
		if _, back, ok := strings.Cut(p.TypeLine, "//"); ok && scryfall.PrimaryType(back) == scryfall.TypeLand {
			if i, seen := backFaceIndex[e.Name]; seen {
				stats.BackFaceLands[i].Quantity += quantity
			} else {
				backFaceIndex[e.Name] = len(stats.BackFaceLands)
				stats.BackFaceLands = append(stats.BackFaceLands, BackFaceCard{Name: e.Name, ScryfallID: e.ScryfallID, Quantity: quantity})
			}
		}
		totalManaValue += p.ManaValue * float64(quantity)
		addToCurve(curve, cardIndex, int(p.ManaValue), e, t, isPermanent(p.TypeLine))

		if len(p.Colors) == 0 {
			stats.ColorBreakdown["C"] += quantity
		} else {
			for _, color := range p.Colors {
				stats.ColorBreakdown[color] += quantity
			}
		}
	}

	stats.NonlandCount = nonlandCount
	stats.TotalManaValue = totalManaValue
	stats.GameChangerCount = len(gameChangers)
	if nonlandCount > 0 {
		stats.AverageManaValue = totalManaValue / float64(nonlandCount)
	}

	sort.SliceStable(stats.BackFaceLands, func(i, j int) bool {
		return strings.ToLower(stats.BackFaceLands[i].Name) < strings.ToLower(stats.BackFaceLands[j].Name)
	})

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

func roundToCents(v Value) Value {
	return Value{EUR: math.Round(v.EUR*100) / 100, USD: math.Round(v.USD*100) / 100}
}

func isPermanent(typeLine string) bool {
	front, _, _ := strings.Cut(typeLine, "//")
	return !strings.Contains(front, "Instant") && !strings.Contains(front, "Sorcery")
}

func addToCurve(curve map[int]*ManaCurveBucket, cardIndex map[int]map[string]int, mv int, e deck.DeckEntry, cardType string, permanent bool) {
	bucket, ok := curve[mv]
	if !ok {
		bucket = &ManaCurveBucket{ManaValue: mv, Cards: []ManaCurveCard{}}
		curve[mv] = bucket
		cardIndex[mv] = make(map[string]int)
	}
	quantity := e.Quantity()
	bucket.Count += quantity
	if permanent {
		bucket.Permanents += quantity
	} else {
		bucket.NonPermanents += quantity
	}
	if i, ok := cardIndex[mv][e.Name]; ok {
		bucket.Cards[i].Quantity += quantity
		return
	}
	cardIndex[mv][e.Name] = len(bucket.Cards)
	bucket.Cards = append(bucket.Cards, ManaCurveCard{Name: e.Name, ScryfallID: e.ScryfallID, Quantity: quantity, Type: cardType})
}
