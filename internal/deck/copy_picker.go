package deck

import (
	"sort"
	"strings"
)

// copyPicker hands out owned copies for a deck, each at most once, skipping
// copies already in it: copies in unlocked storages before those in storages
// locked against the deck (left out unless includeLocked), then copies in no
// deck before copies used by another deck, then the requested finish first.
type copyPicker struct {
	byPrinting map[string][]OwnedCopy
	byName     map[string][]OwnedCopy
	used       map[int]bool
}

func newCopyPicker(copies []OwnedCopy, includeLocked bool) *copyPicker {
	p := &copyPicker{byPrinting: make(map[string][]OwnedCopy), byName: make(map[string][]OwnedCopy), used: make(map[int]bool)}
	for _, c := range copies {
		if c.LockedAgainstTarget && !includeLocked {
			continue
		}
		p.used[c.ID] = c.InTargetDeck
		key := strings.ToLower(c.ScryfallID)
		p.byPrinting[key] = append(p.byPrinting[key], c)
		full, front := CardNameKey(c.Name), CardNameKey(frontFaceName(c.Name))
		p.byName[full] = append(p.byName[full], c)
		if front != full {
			p.byName[front] = append(p.byName[front], c)
		}
	}
	return p
}

func frontFaceName(name string) string {
	front, _, _ := strings.Cut(strings.ReplaceAll(name, "//", "/"), "/")
	return strings.TrimSpace(front)
}

func (p *copyPicker) take(line PlacementLine) []int {
	var copies []OwnedCopy
	if line.AnyPrinting {
		copies = p.byName[CardNameKey(line.Printing.Name)]
	} else {
		copies = p.byPrinting[strings.ToLower(line.Printing.ScryfallID)]
	}
	candidates := append([]OwnedCopy(nil), copies...)
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.LockedAgainstTarget != b.LockedAgainstTarget {
			return !a.LockedAgainstTarget
		}
		if a.InDeck != b.InDeck {
			return !a.InDeck
		}
		if (a.Foil == line.Foil) != (b.Foil == line.Foil) {
			return a.Foil == line.Foil
		}
		return a.ID < b.ID
	})
	ids := make([]int, 0, line.Quantity)
	for _, c := range candidates {
		if len(ids) == line.Quantity {
			break
		}
		if p.used[c.ID] || !line.Proxy.accepts(c.Proxy) {
			continue
		}
		p.used[c.ID] = true
		ids = append(ids, c.ID)
	}
	return ids
}
