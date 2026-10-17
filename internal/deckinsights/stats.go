package deckinsights

import (
	"math"
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/scryfall"
)

func computeStats(entries []deck.DeckEntry, scryfallByID map[string]scryfall.Card, usdPerEUR float64) DeckStats {
	stats := DeckStats{
		ManaCurve:      []ManaCurveBucket{},
		BackFaceLands:  []BackFaceCard{},
		ColorBreakdown: make(map[string]int),
		TypeBreakdown:  make(map[string]int),
	}

	curve := make(map[int]*ManaCurveBucket)
	cardIndex := make(map[int]map[string]int)
	backFaceIndex := make(map[string]int)
	var totalManaValue float64
	var nonlandCount int

	for _, e := range entries {
		quantity := e.Quantity()
		stats.CardCount += quantity

		sc, ok := scryfallByID[e.ScryfallID]
		if !ok {
			stats.TypeBreakdown["Unknown"] += quantity
			continue
		}

		if !e.Proxy {
			eur, usd := sc.Prices.For(e.Foil, usdPerEUR)
			stats.OwnedValue.EUR += eur * float64(e.OwnedQuantity())
			stats.OwnedValue.USD += usd * float64(e.OwnedQuantity())
			stats.PendingValue.EUR += eur * float64(e.PendingQuantity)
			stats.PendingValue.USD += usd * float64(e.PendingQuantity)
		}

		t := scryfall.PrimaryType(sc.TypeLine)
		stats.TypeBreakdown[t] += quantity

		if t == "Land" {
			stats.LandCount += quantity
			continue
		}

		nonlandCount += quantity
		if _, back, ok := strings.Cut(sc.TypeLine, "//"); ok && scryfall.PrimaryType(back) == scryfall.TypeLand {
			if i, seen := backFaceIndex[e.Name]; seen {
				stats.BackFaceLands[i].Quantity += quantity
			} else {
				backFaceIndex[e.Name] = len(stats.BackFaceLands)
				stats.BackFaceLands = append(stats.BackFaceLands, BackFaceCard{Name: e.Name, ScryfallID: e.ScryfallID, Quantity: quantity})
			}
		}
		totalManaValue += sc.CMC * float64(quantity)
		addToCurve(curve, cardIndex, int(sc.CMC), e, t, isPermanent(sc.TypeLine))

		if len(sc.Colors) == 0 {
			stats.ColorBreakdown["C"] += quantity
		} else {
			for _, color := range sc.Colors {
				stats.ColorBreakdown[color] += quantity
			}
		}
	}

	stats.Value = Value{
		EUR: roundToCents(stats.OwnedValue.EUR + stats.PendingValue.EUR),
		USD: roundToCents(stats.OwnedValue.USD + stats.PendingValue.USD),
	}
	stats.OwnedValue = Value{EUR: roundToCents(stats.OwnedValue.EUR), USD: roundToCents(stats.OwnedValue.USD)}
	stats.PendingValue = Value{EUR: roundToCents(stats.PendingValue.EUR), USD: roundToCents(stats.PendingValue.USD)}
	stats.NonlandCount = nonlandCount
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

func entriesValue(entries []deck.DeckEntry, scryfallByID map[string]scryfall.Card, usdPerEUR float64) Value {
	var total Value
	for _, e := range entries {
		sc, ok := scryfallByID[e.ScryfallID]
		if !ok || e.Proxy {
			continue
		}
		eur, usd := sc.Prices.For(e.Foil, usdPerEUR)
		total.EUR += eur * float64(e.Quantity())
		total.USD += usd * float64(e.Quantity())
	}
	return Value{EUR: roundToCents(total.EUR), USD: roundToCents(total.USD)}
}

func roundToCents(amount float64) float64 {
	return math.Round(amount*100) / 100
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
