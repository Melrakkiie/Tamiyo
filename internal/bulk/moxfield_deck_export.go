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

var ErrUnknownExportFormat = errors.New("format must be one of: moxfield, plain, arena, tamiyo")

const (
	DeckExportMoxfield = "moxfield"
	DeckExportPlain    = "plain"
	DeckExportArena    = "arena"
	DeckExportTamiyo   = "tamiyo"
)

type deckExportEntry struct {
	key       moxfieldGroupKey
	board     string
	quantity  int
	commander bool
}

type boardKey struct {
	card  moxfieldGroupKey
	board string
}

type deckExportBoards struct {
	main        []deckExportEntry
	sideboard   []deckExportEntry
	considering []deckExportEntry
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

	boards, err := s.deckExportEntries(ctx, userID, d)
	if err != nil {
		return err
	}

	switch format {
	case DeckExportPlain:
		return writePlainDeck(w, boards)
	case DeckExportArena:
		return writeArenaDeck(w, boards)
	default:
		return writeMoxfieldDeck(w, boards)
	}
}

func (s *Service) deckExportEntries(ctx context.Context, userID string, d deck.Deck) (deckExportBoards, error) {
	cards, err := s.decks.GetDeckCards(ctx, userID, d.ID, "updated", true)
	if err != nil {
		return deckExportBoards{}, fmt.Errorf("loading deck cards: %w", err)
	}
	pending, err := s.decks.GetPendingCards(ctx, userID, d.ID)
	if err != nil {
		return deckExportBoards{}, fmt.Errorf("loading pending cards: %w", err)
	}

	var commanderKey *boardKey
	quantities := make(map[boardKey]int)
	var order []boardKey
	add := func(key boardKey, quantity int, commander bool) {
		if commander {
			commanderKey = &key
		}
		if _, ok := quantities[key]; !ok {
			order = append(order, key)
		}
		quantities[key] += quantity
	}
	for _, c := range cards {
		key := boardKey{moxfieldGroupKey{Name: c.Name, SetCode: c.SetCode, CollectorNumber: c.CollectorNumber, Foil: c.Foil}, c.Board}
		add(key, 1, d.CommanderID != nil && *d.CommanderID == c.ID)
	}
	for _, p := range pending {
		key := boardKey{moxfieldGroupKey{Name: p.Name, SetCode: p.SetCode, CollectorNumber: p.CollectorNumber, Foil: p.Foil}, p.Board}
		add(key, p.Quantity, d.CommanderPendingID != nil && *d.CommanderPendingID == p.ID)
	}

	entries := make([]deckExportEntry, 0, len(order))
	for _, key := range order {
		entries = append(entries, deckExportEntry{
			key:       key.card,
			board:     key.board,
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

	var boards deckExportBoards
	for _, e := range entries {
		switch e.board {
		case deck.BoardSideboard:
			boards.sideboard = append(boards.sideboard, e)
		case deck.BoardConsidering:
			boards.considering = append(boards.considering, e)
		default:
			boards.main = append(boards.main, e)
		}
	}
	return boards, nil
}

func writeSection(w io.Writer, header string, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n%s\n", header); err != nil {
		return fmt.Errorf("writing section: %w", err)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("writing line: %w", err)
		}
	}
	return nil
}

func moxfieldLines(entries []deckExportEntry) []string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		foil := ""
		if e.key.Foil {
			foil = " *F*"
		}
		lines = append(lines, fmt.Sprintf("%d %s (%s) %s%s", e.quantity, e.key.Name, e.key.SetCode, e.key.CollectorNumber, foil))
	}
	return lines
}

func writeMoxfieldDeck(w io.Writer, boards deckExportBoards) error {
	for _, line := range moxfieldLines(boards.main) {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("writing line: %w", err)
		}
	}
	if err := writeSection(w, "SIDEBOARD:", moxfieldLines(boards.sideboard)); err != nil {
		return err
	}
	return writeSection(w, "MAYBEBOARD:", moxfieldLines(boards.considering))
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

func plainLines(entries []deckExportEntry, name func(string) string) []string {
	counts := countByName(entries, name)
	lines := make([]string, 0, len(counts))
	for _, c := range counts {
		lines = append(lines, fmt.Sprintf("%d %s", c.quantity, c.name))
	}
	return lines
}

func samePlainName(name string) string {
	return name
}

func writePlainDeck(w io.Writer, boards deckExportBoards) error {
	for _, line := range plainLines(boards.main, samePlainName) {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("writing line: %w", err)
		}
	}
	if err := writeSection(w, "Sideboard", plainLines(boards.sideboard, samePlainName)); err != nil {
		return err
	}
	return writeSection(w, "Maybeboard", plainLines(boards.considering, samePlainName))
}

func arenaName(name string) string {
	if strings.Contains(name, "//") {
		return name
	}
	return strings.ReplaceAll(name, " / ", " // ")
}

func writeArenaDeck(w io.Writer, boards deckExportBoards) error {
	counts := countByName(boards.main, arenaName)
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
	return writeSection(w, "Sideboard", plainLines(boards.sideboard, arenaName))
}
