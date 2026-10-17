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
	BasicLands  bool
}

var basicLandNames = map[string]bool{
	"Plains":                true,
	"Island":                true,
	"Swamp":                 true,
	"Mountain":              true,
	"Forest":                true,
	"Wastes":                true,
	"Snow-Covered Plains":   true,
	"Snow-Covered Island":   true,
	"Snow-Covered Swamp":    true,
	"Snow-Covered Mountain": true,
	"Snow-Covered Forest":   true,
	"Snow-Covered Wastes":   true,
}

func isBasicLand(name string) bool {
	return basicLandNames[name]
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

	entries, err := s.entriesInBoards(ctx, userID, d.ID, boards)
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
	for _, group := range deck.GroupByCardName(entries) {
		name := arenaName(group[0].Name)
		if !opts.BasicLands && isBasicLand(group[0].Name) {
			continue
		}
		for _, e := range group {
			quantity := e.Quantity()
			if opts.OnlyPending {
				quantity = e.PendingQuantity()
			}
			if quantity == 0 {
				continue
			}
			line := name
			if setName := setNames[strings.ToLower(e.SetCode)]; setName != "" {
				line += " (" + setName + ")"
			}
			if _, seen := quantities[line]; !seen {
				lines = append(lines, line)
			}
			quantities[line] += quantity
		}
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
