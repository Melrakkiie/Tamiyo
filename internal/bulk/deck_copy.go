package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/storage"
)

const (
	CollectAll     = "all"
	CollectMissing = "missing"
	CollectPending = "pending"

	copySuffix = " (copie)"
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
	source, err := s.visibleDeck(ctx, userID, deckID)
	if err != nil {
		return DuplicateSummary{}, err
	}

	var file bytes.Buffer
	lines, err := s.writeTamiyoDeck(ctx, source.ownerID, source.deck.ID, true, &file)
	if err != nil {
		return DuplicateSummary{}, err
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = source.deck.Name + copySuffix
	}
	visibility := req.Visibility
	if visibility == "" {
		visibility = deck.VisibilityPrivate
	}
	created, err := s.decks.CreateDeck(ctx, userID, deck.Deck{
		FolderID:             req.FolderID,
		Name:                 name,
		Format:               source.deck.Format,
		Visibility:           visibility,
		Bracket:              source.deck.Bracket,
		BackgroundScryfallID: source.deck.BackgroundScryfallID,
	})
	if err != nil {
		return DuplicateSummary{}, fmt.Errorf("creating the copy: %w", err)
	}

	if lines == 0 {
		return DuplicateSummary{DeckID: created.ID, Summary: Summary{DecksCreated: 1}}, nil
	}

	summary, err := s.ImportIntoDeck(ctx, userID, created.ID, DeckImportOptions{}, &file)
	if err != nil {
		if deleteErr := s.decks.DeleteDeck(ctx, userID, created.ID); deleteErr != nil {
			return DuplicateSummary{}, errors.Join(err, fmt.Errorf("removing the unfinished copy: %w", deleteErr))
		}
		return DuplicateSummary{}, err
	}
	summary.DecksCreated = 1
	return DuplicateSummary{DeckID: created.ID, Summary: summary}, nil
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

func entryCard(e deck.DeckEntry, storageID *int) card.Card {
	return card.Card{
		Name: e.Name, ScryfallID: e.ScryfallID, SetCode: e.SetCode, CollectorNumber: e.CollectorNumber, Foil: e.Foil, Proxy: e.Proxy,
		StorageID: storageID, ManaValue: e.ManaValue, Colors: e.Colors, CardType: e.CardType, ColorIdentity: e.ColorIdentity,
	}
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

	entries, err := s.entriesInBoards(ctx, source.ownerID, source.deck.ID, boards)
	if err != nil {
		return Summary{}, err
	}

	collected := make([]collectedEntry, len(entries))
	for i, e := range entries {
		collected[i] = collectedEntry{entry: e, copies: e.Quantity()}
		if source.mine {
			collected[i].copies = e.OwnedQuantity()
		}
	}
	if req.Mode == CollectMissing {
		if err = s.withoutOwnedCopies(ctx, userID, collected); err != nil {
			return Summary{}, err
		}
	}

	var summary Summary
	if source.mine {
		s.commitEntriesPending(ctx, userID, source.deck.ID, req.StorageID, entries, &summary)
	}
	if req.Mode == CollectPending {
		return summary, nil
	}
	for _, c := range collected {
		for range c.copies {
			if _, err := s.cards.CreateCard(ctx, userID, entryCard(c.entry, req.StorageID)); err != nil {
				summary.CardsSkipped++
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not create %q: %v", c.entry.Name, err))
				continue
			}
			summary.CardsCreated++
		}
	}
	return summary, nil
}

type collectedEntry struct {
	entry  deck.DeckEntry
	copies int
}

func (s *Service) withoutOwnedCopies(ctx context.Context, userID string, collected []collectedEntry) error {
	keys := make([]string, 0, len(collected))
	seen := make(map[string]bool)
	for _, c := range collected {
		key := deck.CardNameKey(c.entry.Name)
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	owned, err := s.decks.CountCopiesByName(ctx, userID, keys)
	if err != nil {
		return fmt.Errorf("counting owned copies: %w", err)
	}

	for i := range collected {
		key := deck.CardNameKey(collected[i].entry.Name)
		covered := min(owned[key], collected[i].copies)
		owned[key] -= covered
		collected[i].copies -= covered
	}
	return nil
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
			apierr.Mapping{Err: ErrScryfallUnavailable, Status: http.StatusBadGateway},
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
		)
		return
	}

	ctx.JSON(http.StatusOK, summary)
}
