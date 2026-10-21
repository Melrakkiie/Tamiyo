package bulk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
)

var ErrUnknownExportFormat = errors.New("format must be one of: moxfield, plain, arena, tamiyo, cardmarket")

const (
	DeckExportMoxfield = "moxfield"
	DeckExportPlain    = "plain"
	DeckExportArena    = "arena"
	DeckExportTamiyo   = "tamiyo"
)

type deckExportBoards struct {
	main        []deck.DeckEntry
	sideboard   []deck.DeckEntry
	considering []deck.DeckEntry
}

func (s *Service) ExportDeck(ctx context.Context, userID string, deckID string, opts DeckExportOptions, w io.Writer) error {
	format := opts.Format
	if format != DeckExportMoxfield && format != DeckExportPlain && format != DeckExportArena {
		return ErrUnknownExportFormat
	}

	own, err := s.decks.ReadDeck(ctx, userID, deckID, deck.ReachOwn)
	if err != nil {
		return err
	}
	return s.writeReadDeck(ctx, own, opts, w)
}

// writeReadDeck writes a read deck in opts.Format for both the own and the
// shared exports. The cardmarket boards are validated after the read here,
// so the own export also validates them before its read.
func (s *Service) writeReadDeck(ctx context.Context, read deck.ReadDeck, opts DeckExportOptions, w io.Writer) error {
	switch opts.Format {
	case DeckExportTamiyo:
		return writeTamiyoDeck(w, read, opts.WithTags)
	case DeckExportCardmarket:
		boards, err := cardmarketBoards(opts)
		if err != nil {
			return err
		}
		return s.writeCardmarketDeck(ctx, deck.EntriesInBoards(read.Entries, boardList(boards)), opts, w)
	default:
		var tags map[string][]string
		if opts.WithTags {
			tags = read.Tags
		}
		return writeTextDeck(w, opts.Format, read.Entries, tags)
	}
}

// writeTextDeck writes a moxfield, plain or arena list; only moxfield shows
// the tags, keyed by deck.CardNameKey.
func writeTextDeck(w io.Writer, format string, entries []deck.DeckEntry, tags map[string][]string) error {
	boards := splitByBoard(entries)
	switch format {
	case DeckExportPlain:
		return writePlainDeck(w, boards)
	case DeckExportArena:
		return writeArenaDeck(w, boards)
	default:
		return writeMoxfieldDeck(w, boards, tags)
	}
}

func (s *Service) ExportSharedDeck(ctx context.Context, deckID string, opts DeckExportOptions, w io.Writer) error {
	if _, known := deckExportFilenames[opts.Format]; !known {
		return ErrUnknownExportFormat
	}
	shared, err := s.decks.ReadDeck(ctx, "", deckID, deck.ReachShared)
	if err != nil {
		return err
	}
	return s.writeReadDeck(ctx, shared, opts, w)
}

func splitByBoard(entries []deck.DeckEntry) deckExportBoards {
	var boards deckExportBoards
	for _, e := range entries {
		switch e.Board {
		case deck.BoardSideboard:
			boards.sideboard = append(boards.sideboard, e)
		case deck.BoardConsidering:
			boards.considering = append(boards.considering, e)
		default:
			boards.main = append(boards.main, e)
		}
	}
	return boards
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

func moxfieldLines(entries []deck.DeckEntry, tags map[string][]string) []string {
	groups := deck.GroupByPrinting(entries)
	lines := make([]string, 0, len(groups))
	for _, g := range groups {
		e := g[0]
		foil := ""
		if e.Foil {
			foil = " *F*"
		}
		line := fmt.Sprintf("%d %s (%s) %s%s", g.Quantity(), e.Name, e.SetCode, e.CollectorNumber, foil)
		for _, tag := range tags[deck.CardNameKey(e.Name)] {
			line += " #" + tag
		}
		lines = append(lines, line)
	}
	return lines
}

func writeMoxfieldDeck(w io.Writer, boards deckExportBoards, tags map[string][]string) error {
	for _, line := range moxfieldLines(boards.main, tags) {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("writing line: %w", err)
		}
	}
	if err := writeSection(w, "SIDEBOARD:", moxfieldLines(boards.sideboard, tags)); err != nil {
		return err
	}
	return writeSection(w, "MAYBEBOARD:", moxfieldLines(boards.considering, tags))
}

func splitCommanders(entries []deck.DeckEntry) (commanders, others []deck.DeckEntry) {
	for _, e := range entries {
		if e.IsCommander() {
			commanders = append(commanders, e)
		} else {
			others = append(others, e)
		}
	}
	return commanders, others
}

func nameLines(entries []deck.DeckEntry, name func(string) string) []string {
	groups := deck.GroupByCardName(entries)
	lines := make([]string, 0, len(groups))
	for _, g := range groups {
		lines = append(lines, fmt.Sprintf("%d %s", g.Quantity(), name(g[0].Name)))
	}
	return lines
}

func plainLines(entries []deck.DeckEntry, name func(string) string) []string {
	commanders, others := splitCommanders(entries)
	return append(nameLines(commanders, name), nameLines(others, name)...)
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
	commanders, others := splitCommanders(boards.main)

	var b strings.Builder
	if len(commanders) > 0 {
		b.WriteString("Commander\n")
		for _, line := range nameLines(commanders, arenaName) {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Deck\n")
	for _, line := range nameLines(others, arenaName) {
		b.WriteString(line + "\n")
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("writing deck: %w", err)
	}
	return writeSection(w, "Sideboard", plainLines(boards.sideboard, arenaName))
}
