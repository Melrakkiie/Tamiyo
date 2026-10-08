package bulk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
)

var ErrUnknownExportFormat = errors.New("format must be one of: moxfield, plain, arena")

const (
	DeckExportMoxfield = "moxfield"
	DeckExportPlain    = "plain"
	DeckExportArena    = "arena"
)

type deckExportEntry struct {
	key       moxfieldGroupKey
	quantity  int
	commander bool
}

func (s *Service) ExportDeck(ctx context.Context, userID string, deckID string, format string, w io.Writer) error {
	if format != DeckExportMoxfield && format != DeckExportPlain && format != DeckExportArena {
		return ErrUnknownExportFormat
	}

	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return ErrDeckNotFound
		}
		return err
	}

	entries, err := s.deckExportEntries(ctx, userID, d)
	if err != nil {
		return err
	}

	switch format {
	case DeckExportPlain:
		return writePlainDeck(w, entries)
	case DeckExportArena:
		return writeArenaDeck(w, entries)
	default:
		return writeMoxfieldDeck(w, entries)
	}
}

func (s *Service) deckExportEntries(ctx context.Context, userID string, d deck.Deck) ([]deckExportEntry, error) {
	cards, err := s.decks.GetDeckCards(ctx, userID, d.ID, "updated", true)
	if err != nil {
		return nil, fmt.Errorf("loading deck cards: %w", err)
	}
	pending, err := s.decks.GetPendingCards(ctx, userID, d.ID)
	if err != nil {
		return nil, fmt.Errorf("loading pending cards: %w", err)
	}

	var commanderKey *moxfieldGroupKey
	quantities := make(map[moxfieldGroupKey]int)
	var order []moxfieldGroupKey
	add := func(key moxfieldGroupKey, quantity int, commander bool) {
		if commander {
			commanderKey = &key
		}
		if _, ok := quantities[key]; !ok {
			order = append(order, key)
		}
		quantities[key] += quantity
	}
	for _, c := range cards {
		key := moxfieldGroupKey{Name: c.Name, SetCode: c.SetCode, CollectorNumber: c.CollectorNumber, Foil: c.Foil}
		add(key, 1, d.CommanderID != nil && *d.CommanderID == c.ID)
	}
	for _, p := range pending {
		key := moxfieldGroupKey{Name: p.Name, SetCode: p.SetCode, CollectorNumber: p.CollectorNumber, Foil: p.Foil}
		add(key, p.Quantity, d.CommanderPendingID != nil && *d.CommanderPendingID == p.ID)
	}

	entries := make([]deckExportEntry, 0, len(order))
	for _, key := range order {
		entries = append(entries, deckExportEntry{
			key:       key,
			quantity:  quantities[key],
			commander: commanderKey != nil && key == *commanderKey,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.commander != b.commander {
			return a.commander
		}
		if a.key.Name != b.key.Name {
			return a.key.Name < b.key.Name
		}
		if a.key.SetCode != b.key.SetCode {
			return a.key.SetCode < b.key.SetCode
		}
		if a.key.CollectorNumber != b.key.CollectorNumber {
			return a.key.CollectorNumber < b.key.CollectorNumber
		}
		return !a.key.Foil && b.key.Foil
	})
	return entries, nil
}

func writeMoxfieldDeck(w io.Writer, entries []deckExportEntry) error {
	for _, e := range entries {
		foil := ""
		if e.key.Foil {
			foil = " *F*"
		}
		if _, err := fmt.Fprintf(w, "%d %s (%s) %s%s\n", e.quantity, e.key.Name, e.key.SetCode, e.key.CollectorNumber, foil); err != nil {
			return fmt.Errorf("writing line: %w", err)
		}
	}
	return nil
}

type nameCount struct {
	name      string
	quantity  int
	commander bool
}

func countByName(entries []deckExportEntry, name func(string) string) []nameCount {
	var counts []nameCount
	index := make(map[string]int)
	for _, e := range entries {
		key := name(e.key.Name)
		if e.commander {
			key = "\x00" + key
		}
		if i, ok := index[key]; ok {
			counts[i].quantity += e.quantity
			continue
		}
		index[key] = len(counts)
		counts = append(counts, nameCount{name: name(e.key.Name), quantity: e.quantity, commander: e.commander})
	}
	return counts
}

func writePlainDeck(w io.Writer, entries []deckExportEntry) error {
	for _, c := range countByName(entries, func(name string) string { return name }) {
		if _, err := fmt.Fprintf(w, "%d %s\n", c.quantity, c.name); err != nil {
			return fmt.Errorf("writing line: %w", err)
		}
	}
	return nil
}

func arenaName(name string) string {
	if strings.Contains(name, "//") {
		return name
	}
	return strings.ReplaceAll(name, " / ", " // ")
}

func writeArenaDeck(w io.Writer, entries []deckExportEntry) error {
	counts := countByName(entries, arenaName)
	var commanders, others []nameCount
	for _, c := range counts {
		if c.commander {
			commanders = append(commanders, c)
		} else {
			others = append(others, c)
		}
	}

	var b strings.Builder
	if len(commanders) > 0 {
		b.WriteString("Commander\n")
		for _, c := range commanders {
			fmt.Fprintf(&b, "%d %s\n", c.quantity, c.name)
		}
		b.WriteString("\n")
	}
	b.WriteString("Deck\n")
	for _, c := range others {
		fmt.Fprintf(&b, "%d %s\n", c.quantity, c.name)
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("writing deck: %w", err)
	}
	return nil
}
