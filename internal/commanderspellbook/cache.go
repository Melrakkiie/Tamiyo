package commanderspellbook

import (
	"container/list"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	cacheTTL        = 24 * time.Hour
	cacheMaxEntries = 500
)

// comboCache keeps the combos found for a card list for cacheTTL. It holds at
// most maxEntries lists and drops the oldest stored first. It is safe for
// concurrent use.
type comboCache struct {
	now        func() time.Time
	maxEntries int

	mu      sync.Mutex
	entries map[string]*list.Element // value: *cacheEntry
	order   *list.List               // front is the oldest entry
}

type cacheEntry struct {
	key      string
	combos   []Combo
	storedAt time.Time
}

func newComboCache(now func() time.Time, maxEntries int) *comboCache {
	return &comboCache{
		now:        now,
		maxEntries: maxEntries,
		entries:    make(map[string]*list.Element),
		order:      list.New(),
	}
}

// cacheKey is the same for a list given in any order. Commanders and main
// cards are kept apart: the same name does not mean the same thing in both.
func cacheKey(commanders, main []string) string {
	sortedCommanders := slices.Sorted(slices.Values(commanders))
	sortedMain := slices.Sorted(slices.Values(main))
	return strings.Join(sortedCommanders, "\x00") + "\x01" + strings.Join(sortedMain, "\x00")
}

func (c *comboCache) get(key string) ([]Combo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	entry := element.Value.(*cacheEntry)
	if c.now().Sub(entry.storedAt) >= cacheTTL {
		c.remove(element)
		return nil, false
	}
	return cloneCombos(entry.combos), true
}

func (c *comboCache) put(key string, combos []Combo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[key]; ok {
		c.remove(existing)
	}
	c.entries[key] = c.order.PushBack(&cacheEntry{key: key, combos: cloneCombos(combos), storedAt: c.now()})
	for c.order.Len() > c.maxEntries {
		c.remove(c.order.Front())
	}
}

func (c *comboCache) remove(element *list.Element) {
	delete(c.entries, element.Value.(*cacheEntry).key)
	c.order.Remove(element)
}

// cloneCombos keeps callers from altering what the cache holds.
func cloneCombos(combos []Combo) []Combo {
	cloned := make([]Combo, len(combos))
	for i, combo := range combos {
		cloned[i] = Combo{ID: combo.ID, Cards: slices.Clone(combo.Cards), Results: slices.Clone(combo.Results)}
	}
	return cloned
}
