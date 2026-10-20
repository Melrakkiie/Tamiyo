package bulk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

const (
	CollectAll     = "all"
	CollectMissing = "missing"
	CollectPending = "pending"
)

var (
	ErrUnknownCollectMode = errors.New("mode must be one of: all, missing, pending")
	ErrCollectModeNotHere = errors.New("use pending or all on your own decks, missing or all on other decks")
	ErrNoBoards           = errors.New("boards must list at least one of: main, sideboard, considering")
)

type visibleDeck struct {
	ownerID string
	deck    deck.Deck
	mine    bool
}

func (s *Service) visibleDeck(ctx context.Context, userID string, deckID string) (visibleDeck, error) {
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err == nil {
		return visibleDeck{ownerID: userID, deck: d, mine: true}, nil
	}
	if !errors.Is(err, deck.ErrNotFound) {
		return visibleDeck{}, err
	}
	ownerID, shared, err := s.decks.GetSharedDeck(ctx, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return visibleDeck{}, ErrDeckNotFound
		}
		return visibleDeck{}, err
	}
	return visibleDeck{ownerID: ownerID, deck: shared}, nil
}

type DuplicateRequest struct {
	Name       string `json:"name"`
	FolderID   *int   `json:"folder_id"`
	Visibility string `json:"visibility"`
}

type DuplicateSummary struct {
	DeckID  string  `json:"deck_id"`
	Summary Summary `json:"summary"`
}

func (s *Service) DuplicateDeck(ctx context.Context, userID string, deckID string, req DuplicateRequest) (DuplicateSummary, error) {
	result, err := s.decks.DuplicateDeck(ctx, userID, deckID, deck.DuplicateRequest{
		Name: req.Name, FolderID: req.FolderID, Visibility: req.Visibility,
	})
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return DuplicateSummary{}, ErrDeckNotFound
		}
		return DuplicateSummary{}, err
	}
	summary := Summary{DecksCreated: 1}
	summary.CardsLinked = result.Placement.CardsLinked
	summary.CardsPending = result.Placement.CardsPending
	for _, skipped := range result.Placement.SkippedTags {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not tag %q: %v", skipped.CardName, skipped.Err))
	}
	return DuplicateSummary{DeckID: result.Deck.ID, Summary: summary}, nil
}

type CollectRequest struct {
	Mode      string   `json:"mode"`
	StorageID *int     `json:"storage_id"`
	Boards    []string `json:"boards"`
}

func wantedBoards(boards []string) (map[string]bool, error) {
	wanted := make(map[string]bool, len(boards))
	for _, b := range boards {
		if b != deck.BoardMain && b != deck.BoardSideboard && b != deck.BoardConsidering {
			return nil, ErrNoBoards
		}
		wanted[b] = true
	}
	if len(wanted) == 0 {
		return nil, ErrNoBoards
	}
	return wanted, nil
}

func boardOf(board string) string {
	if board == "" {
		return deck.BoardMain
	}
	return board
}

func (s *Service) entriesInBoards(ctx context.Context, ownerID string, deckID string, boards map[string]bool) ([]deck.DeckEntry, error) {
	content, err := s.decks.GetDeckContent(ctx, ownerID, deckID)
	if err != nil {
		return nil, fmt.Errorf("loading deck content: %w", err)
	}
	entries := make([]deck.DeckEntry, 0, len(content.Entries))
	for _, e := range content.Entries {
		if boards[boardOf(e.Board)] {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func (s *Service) CollectDeck(ctx context.Context, userID string, deckID string, req CollectRequest) (Summary, error) {
	if req.Mode != CollectAll && req.Mode != CollectMissing && req.Mode != CollectPending {
		return Summary{}, ErrUnknownCollectMode
	}
	boards, err := wantedBoards(req.Boards)
	if err != nil {
		return Summary{}, err
	}
	source, err := s.visibleDeck(ctx, userID, deckID)
	if err != nil {
		return Summary{}, err
	}
	if (req.Mode == CollectPending && !source.mine) || (req.Mode == CollectMissing && source.mine) {
		return Summary{}, ErrCollectModeNotHere
	}
	if req.StorageID != nil {
		if _, err := s.storages.GetStorage(ctx, userID, *req.StorageID); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return Summary{}, ErrTargetStorageNotFound
			}
			return Summary{}, err
		}
	}

	chosen := make([]string, 0, len(boards))
	for _, b := range []string{deck.BoardMain, deck.BoardSideboard, deck.BoardConsidering} {
		if boards[b] {
			chosen = append(chosen, b)
		}
	}

	var result deck.CollectSummary
	if source.mine {
		result, err = s.decks.CollectOwnDeck(ctx, userID, deckID, deck.OwnDeckCollect{
			Boards: chosen, StorageID: req.StorageID, OwnedCopies: req.Mode == CollectAll,
		})
	} else {
		result, err = s.decks.CollectSharedDeck(ctx, userID, deckID, deck.SharedDeckCollect{
			Boards: chosen, StorageID: req.StorageID, MissingOnly: req.Mode == CollectMissing,
		})
	}
	if err != nil {
		return Summary{}, err
	}
	return Summary{CardsCreated: result.CardsCreated, CardsLinked: result.CardsLinked}, nil
}

func (h *Handler) duplicateDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	deckID, valid := deck.ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req DuplicateRequest
	if ctx.Request.ContentLength != 0 {
		if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		if req.FolderID != nil && *req.FolderID <= 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "folder_id must be a positive integer"})
			return
		}
		switch req.Visibility {
		case "", deck.VisibilityPublic, deck.VisibilityUnlisted, deck.VisibilityPrivate:
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "visibility must be one of: public, unlisted, private"})
			return
		}
	}

	result, err := h.service.DuplicateDeck(ctx.Request.Context(), userID, deckID, req)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: deck.ErrTargetFolderNotFound, Status: http.StatusBadRequest, Message: deck.ErrTargetFolderNotFound.Error()},
		)
		return
	}

	ctx.JSON(http.StatusCreated, result)
}

func (h *Handler) collectDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	deckID, valid := deck.ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req CollectRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.StorageID != nil && *req.StorageID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id must be a positive integer"})
		return
	}

	summary, err := h.service.CollectDeck(ctx.Request.Context(), userID, deckID, req)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrUnknownCollectMode, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrCollectModeNotHere, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrNoBoards, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrTargetStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"},
			apierr.Mapping{Err: deck.ErrTargetStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"},
		)
		return
	}

	ctx.JSON(http.StatusOK, summary)
}
