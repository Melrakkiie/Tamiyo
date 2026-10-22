package deck

import (
	"context"
	"sort"
	"strings"
)

type DeckContent struct {
	Deck    Deck
	Entries []DeckEntry
}

// DeckEntry is one line of a deck: the owned and pending copies sharing a
// printing, finish, proxy flag, board and commander slot.
type DeckEntry struct {
	Name            string
	ScryfallID      string
	SetCode         string
	CollectorNumber string
	ManaValue       float64
	Colors          *string
	CardType        *string
	ColorIdentity   *string
	Foil            bool
	Proxy           bool
	Board           string
	Slot            CommanderSlot
	Price           *float64
	GameChanger     bool

	OwnedCardIDs []int
	Pending      []PendingPart
}

// PendingPart is the share of one pending card in an entry: a pending
// commander of several copies is split between its commander entry and a
// regular one, which can also hold other pending cards of the printing.
type PendingPart struct {
	ID       int
	Quantity int
}

func (e DeckEntry) OwnedQuantity() int {
	return len(e.OwnedCardIDs)
}

func (e DeckEntry) PendingQuantity() int {
	total := 0
	for _, p := range e.Pending {
		total += p.Quantity
	}
	return total
}

func (e DeckEntry) PendingIDs() []int {
	ids := make([]int, 0, len(e.Pending))
	for _, p := range e.Pending {
		ids = append(ids, p.ID)
	}
	return ids
}

func (e DeckEntry) Quantity() int {
	return e.OwnedQuantity() + e.PendingQuantity()
}

func (e DeckEntry) IsCommander() bool {
	return e.Slot != SlotNone
}

type contentReader interface {
	FindByID(ctx context.Context, userID string, id string) (Deck, error)
	FindCardsByDeckID(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]DeckCard, error)
	FindPendingCards(ctx context.Context, userID string, deckID string) ([]PendingCard, error)
}

// readDeckContent returns the deck's owned and pending cards merged into
// entries ordered commanders first (by slot), then by board (main, sideboard,
// considering), name, set code, collector number, non-foil before foil and
// real before proxy. Callers rely on this order.
func readDeckContent(ctx context.Context, r contentReader, userID string, deckID string) (DeckContent, error) {
	d, err := r.FindByID(ctx, userID, deckID)
	if err != nil {
		return DeckContent{}, err
	}
	entries, err := loadEntries(ctx, r, userID, d)
	if err != nil {
		return DeckContent{}, err
	}
	return DeckContent{Deck: d, Entries: entries}, nil
}

func loadEntries(ctx context.Context, r contentReader, ownerID string, d Deck) ([]DeckEntry, error) {
	owned, err := r.FindCardsByDeckID(ctx, ownerID, d.ID, "added", false)
	if err != nil {
		return nil, err
	}
	pending, err := r.FindPendingCards(ctx, ownerID, d.ID)
	if err != nil {
		return nil, err
	}
	return mergeDeckEntries(d, owned, pending), nil
}

type entryKey struct {
	scryfallID string
	foil       bool
	proxy      bool
	board      string
	slot       CommanderSlot
}

func (e DeckEntry) key() entryKey {
	return entryKey{strings.ToLower(e.ScryfallID), e.Foil, e.Proxy, e.Board, e.Slot}
}

func mergeDeckEntries(d Deck, owned []DeckCard, pending []PendingCard) []DeckEntry {
	var entries []DeckEntry
	index := make(map[entryKey]int)
	add := func(e DeckEntry) {
		k := e.key()
		i, ok := index[k]
		if !ok {
			index[k] = len(entries)
			entries = append(entries, e)
			return
		}
		entries[i].OwnedCardIDs = append(entries[i].OwnedCardIDs, e.OwnedCardIDs...)
		entries[i].Pending = append(entries[i].Pending, e.Pending...)
	}

	for _, c := range owned {
		slot, _ := d.CardSlot(c.ID)
		e := ownedEntry(c)
		e.Slot = slot
		add(e)
	}
	for _, p := range pending {
		remaining := p.Quantity
		if slot, ok := d.PendingSlot(p.ID); ok {
			e := pendingEntry(p, 1)
			e.Slot = slot
			add(e)
			remaining--
		}
		if remaining > 0 {
			add(pendingEntry(p, remaining))
		}
	}

	sort.SliceStable(entries, func(i, j int) bool { return entryBefore(entries[i], entries[j]) })
	return entries
}

var boardOrder = map[string]int{BoardMain: 0, BoardSideboard: 1, BoardConsidering: 2}

func entryBefore(a, b DeckEntry) bool {
	if a.IsCommander() != b.IsCommander() {
		return a.IsCommander()
	}
	if a.Slot != b.Slot {
		return a.Slot < b.Slot
	}
	if boardOrder[a.Board] != boardOrder[b.Board] {
		return boardOrder[a.Board] < boardOrder[b.Board]
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	if a.SetCode != b.SetCode {
		return a.SetCode < b.SetCode
	}
	if a.CollectorNumber != b.CollectorNumber {
		return a.CollectorNumber < b.CollectorNumber
	}
	if a.Foil != b.Foil {
		return !a.Foil
	}
	return !a.Proxy && b.Proxy
}

func ownedEntry(c DeckCard) DeckEntry {
	return DeckEntry{
		Name:            c.Name,
		ScryfallID:      c.ScryfallID,
		SetCode:         c.SetCode,
		CollectorNumber: c.CollectorNumber,
		ManaValue:       c.ManaValue,
		Colors:          c.Colors,
		CardType:        c.CardType,
		ColorIdentity:   c.ColorIdentity,
		Foil:            c.Foil,
		Proxy:           c.Proxy,
		Board:           c.Board,
		Price:           c.Price,
		GameChanger:     c.GameChanger,
		OwnedCardIDs:    []int{c.ID},
	}
}

func pendingEntry(p PendingCard, quantity int) DeckEntry {
	return DeckEntry{
		Name:            p.Name,
		ScryfallID:      p.ScryfallID,
		SetCode:         p.SetCode,
		CollectorNumber: p.CollectorNumber,
		ManaValue:       p.ManaValue,
		Colors:          p.Colors,
		CardType:        p.CardType,
		ColorIdentity:   p.ColorIdentity,
		Foil:            p.Foil,
		Proxy:           p.Proxy,
		Board:           p.Board,
		Price:           p.Price,
		GameChanger:     p.GameChanger,
		Pending:         []PendingPart{{ID: p.ID, Quantity: quantity}},
	}
}

type PendingTotal struct {
	ID       int
	Name     string
	Quantity int
}

func PendingTotals(entries []DeckEntry) []PendingTotal {
	var totals []PendingTotal
	index := make(map[int]int)
	for _, e := range entries {
		for _, part := range e.Pending {
			i, ok := index[part.ID]
			if !ok {
				index[part.ID] = len(totals)
				totals = append(totals, PendingTotal{ID: part.ID, Name: e.Name})
				i = len(totals) - 1
			}
			totals[i].Quantity += part.Quantity
		}
	}
	return totals
}

type EntryGroup []DeckEntry

func (g EntryGroup) OwnedQuantity() int {
	total := 0
	for _, e := range g {
		total += e.OwnedQuantity()
	}
	return total
}

func (g EntryGroup) PendingQuantity() int {
	total := 0
	for _, e := range g {
		total += e.PendingQuantity()
	}
	return total
}

func (g EntryGroup) Quantity() int {
	return g.OwnedQuantity() + g.PendingQuantity()
}

func GroupByPrinting(entries []DeckEntry) []EntryGroup {
	return groupEntries(entries, func(e DeckEntry) entryKey {
		k := e.key()
		k.proxy = false
		return k
	})
}

func GroupByCardName(entries []DeckEntry) []EntryGroup {
	return groupEntries(entries, func(e DeckEntry) string { return CardNameKey(e.Name) })
}

func groupEntries[K comparable](entries []DeckEntry, keyOf func(DeckEntry) K) []EntryGroup {
	var groups []EntryGroup
	index := make(map[K]int)
	for _, e := range entries {
		k := keyOf(e)
		i, ok := index[k]
		if !ok {
			index[k] = len(groups)
			groups = append(groups, nil)
			i = len(groups) - 1
		}
		groups[i] = append(groups[i], e)
	}
	return groups
}
