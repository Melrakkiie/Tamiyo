package deckinsights

import (
	"context"
	"fmt"
	"sort"

	"Melrakkiie/Tamiyo/internal/deck"
)

type historyReader interface {
	DeckHistoryRows(ctx context.Context, userID string, deckID string) ([]HistoryRow, error)
}

// GetDeckHistory returns the history of the deck viewerID reads within reach.
// shortcut: whole history in one response, paginate if decks reach thousands of rows
func (s *Service) GetDeckHistory(ctx context.Context, viewerID string, deckID string, reach deck.Reach) (DeckHistory, error) {
	read, err := s.readDeck(ctx, viewerID, deckID, reach)
	if err != nil {
		return DeckHistory{}, err
	}
	rows, err := s.store.DeckHistoryRows(ctx, read.OwnerID, read.Deck.ID)
	if err != nil {
		return DeckHistory{}, fmt.Errorf("loading deck history: %w", err)
	}
	return groupHistory(rows), nil
}

var boardOrder = map[string]int{deck.BoardMain: 0, deck.BoardSideboard: 1, deck.BoardConsidering: 2}

// groupHistory arranges rows into days (newest first), boards (main,
// sideboard, considering) and changes (by name).
func groupHistory(rows []HistoryRow) DeckHistory {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Date != b.Date {
			return a.Date > b.Date
		}
		if a.Board != b.Board {
			return boardOrder[a.Board] < boardOrder[b.Board]
		}
		return a.Name < b.Name
	})
	history := DeckHistory{Days: []HistoryDay{}}
	for _, r := range rows {
		if n := len(history.Days); n == 0 || history.Days[n-1].Date != r.Date {
			history.Days = append(history.Days, HistoryDay{Date: r.Date, Boards: []HistoryBoard{}})
		}
		day := &history.Days[len(history.Days)-1]
		if n := len(day.Boards); n == 0 || day.Boards[n-1].Board != r.Board {
			day.Boards = append(day.Boards, HistoryBoard{Board: r.Board})
		}
		board := &day.Boards[len(day.Boards)-1]
		board.Changes = append(board.Changes, HistoryChange{Name: r.Name, Count: r.Count, SetCode: r.SetCode, CollectorNumber: r.CollectorNumber, Foil: r.Foil})
	}
	return history
}
