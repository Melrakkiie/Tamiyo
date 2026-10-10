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

const DeckExportCardmarket = "cardmarket"

type DeckExportOptions struct {
	Format      string
	WithTags    bool
	OnlyPending bool
	Printings   bool
	Boards      []string
}

var defaultCardmarketBoards = []string{deck.BoardMain, deck.BoardSideboard}

func (s *Service) ExportCardmarketDeck(ctx context.Context, userID string, deckID string, opts DeckExportOptions, w io.Writer) error {
	boardNames := opts.Boards
	if len(boardNames) == 0 {
		boardNames = defaultCardmarketBoards
	}
	boards, err := wantedBoards(boardNames)
	if err != nil {
		return err
	}

	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return ErrDeckNotFound
		}
		return err
	}

	entries, err := s.collectEntries(ctx, userID, d.ID, boards)
	if err != nil {
		return err
	}

	var setNames map[string]string
	if opts.Printings {
		if setNames, err = s.scryfall.SetNames(ctx); err != nil {
			return fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
		}
	}

	quantities := make(map[string]int)
	var lines []string
	for _, e := range entries {
		if opts.OnlyPending && e.pendingID == 0 {
			continue
		}
		line := arenaName(e.card.Name)
		if setName := setNames[strings.ToLower(e.card.SetCode)]; setName != "" {
			line += " (" + setName + ")"
		}
		if _, seen := quantities[line]; !seen {
			lines = append(lines, line)
		}
		quantities[line] += e.quantity
	}
	sort.Slice(lines, func(i, j int) bool { return strings.ToLower(lines[i]) < strings.ToLower(lines[j]) })

	var b strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&b, "%d %s\n", quantities[line], line)
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("writing deck: %w", err)
	}
	return nil
}

func (s *Service) exportDeckAs(ctx context.Context, userID string, deckID string, opts DeckExportOptions, w io.Writer) error {
	switch opts.Format {
	case DeckExportTamiyo:
		return s.ExportTamiyoDeck(ctx, userID, deckID, opts.WithTags, w)
	case DeckExportCardmarket:
		return s.ExportCardmarketDeck(ctx, userID, deckID, opts, w)
	default:
		return s.ExportDeck(ctx, userID, deckID, opts, w)
	}
}
