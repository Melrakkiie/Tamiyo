package deck

import (
	"context"
	"errors"
)

var (
	ErrInvalidViewGrouping = errors.New("grouping must be one of: type, color, mana, storage, tag, or null")
	ErrInvalidViewSort     = errors.New("sort must be one of: name, -name, added, -added, updated, -updated, mana_value, -mana_value")
	ErrInvalidViewBoards   = errors.New("collapsed_boards may only contain sideboard and considering")
)

const defaultViewSort = "mana_value"

var defaultViewGrouping = "type"

var viewGroupings = map[string]bool{"type": true, "color": true, "mana": true, "storage": true, "tag": true}

var viewSorts = map[string]bool{
	"name": true, "-name": true,
	"added": true, "-added": true,
	"updated": true, "-updated": true,
	"mana_value": true, "-mana_value": true,
}

var collapsibleBoards = []string{BoardSideboard, BoardConsidering}

type View struct {
	Grouping        *string
	Sort            string
	CollapsedBoards []string
}

type ViewRepository interface {
	FindView(ctx context.Context, userID string, deckID string) (View, bool, error)
	SaveView(ctx context.Context, userID string, deckID string, v View) error
}

func DefaultView() View {
	grouping := defaultViewGrouping
	return View{Grouping: &grouping, Sort: defaultViewSort, CollapsedBoards: defaultCollapsedBoards()}
}

func defaultCollapsedBoards() []string {
	return []string{BoardConsidering}
}

func normalizeCollapsedBoards(boards []string) ([]string, error) {
	if boards == nil {
		return defaultCollapsedBoards(), nil
	}
	wanted := make(map[string]bool, len(boards))
	for _, board := range boards {
		if board != BoardSideboard && board != BoardConsidering {
			return nil, ErrInvalidViewBoards
		}
		wanted[board] = true
	}
	normalized := []string{}
	for _, board := range collapsibleBoards {
		if wanted[board] {
			normalized = append(normalized, board)
		}
	}
	return normalized, nil
}

func (s *Service) GetView(ctx context.Context, userID string, deckID string) (View, error) {
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return View{}, err
	}
	v, found, err := s.repo.FindView(ctx, userID, deckID)
	if err != nil {
		return View{}, err
	}
	if !found {
		return DefaultView(), nil
	}
	return v, nil
}

func (s *Service) SetView(ctx context.Context, userID string, deckID string, v View) (View, error) {
	if v.Grouping != nil && !viewGroupings[*v.Grouping] {
		return View{}, ErrInvalidViewGrouping
	}
	if !viewSorts[v.Sort] {
		return View{}, ErrInvalidViewSort
	}
	boards, err := normalizeCollapsedBoards(v.CollapsedBoards)
	if err != nil {
		return View{}, err
	}
	v.CollapsedBoards = boards
	if _, err := s.repo.FindByID(ctx, userID, deckID); err != nil {
		return View{}, err
	}
	if err := s.repo.SaveView(ctx, userID, deckID, v); err != nil {
		return View{}, err
	}
	return v, nil
}
