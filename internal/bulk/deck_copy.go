package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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

type DuplicateSummary struct {
	DeckID  string  `json:"deck_id"`
	Summary Summary `json:"summary"`
}

func (s *Service) DuplicateDeck(ctx context.Context, userID string, deckID string) (DuplicateSummary, error) {
	source, err := s.visibleDeck(ctx, userID, deckID)
	if err != nil {
		return DuplicateSummary{}, err
	}

	content, err := s.collectEntries(ctx, source.ownerID, source.deck.ID, allBoards)
	if err != nil {
		return DuplicateSummary{}, err
	}

	var file bytes.Buffer
	if err := s.ExportTamiyoDeck(ctx, source.ownerID, source.deck.ID, true, &file); err != nil {
		return DuplicateSummary{}, err
	}

	var folderID *int
	if source.mine {
		folderID = source.deck.FolderID
	}
	created, err := s.decks.CreateDeck(ctx, userID, deck.Deck{
		FolderID:             folderID,
		Name:                 source.deck.Name + copySuffix,
		Format:               source.deck.Format,
		Visibility:           deck.VisibilityPrivate,
		Bracket:              source.deck.Bracket,
		BackgroundScryfallID: source.deck.BackgroundScryfallID,
	})
	if err != nil {
		return DuplicateSummary{}, fmt.Errorf("creating the copy: %w", err)
	}

	if len(content) == 0 {
		return DuplicateSummary{DeckID: created.ID, Summary: Summary{DecksCreated: 1}}, nil
	}

	summary, err := s.ImportIntoDeck(ctx, userID, created.ID, false, &file)
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

var allBoards = map[string]bool{deck.BoardMain: true, deck.BoardSideboard: true, deck.BoardConsidering: true}

type collectEntry struct {
	card      card.Card
	quantity  int
	pendingID int
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

func (s *Service) collectEntries(ctx context.Context, ownerID string, deckID string, boards map[string]bool) ([]collectEntry, error) {
	cards, err := s.decks.GetDeckCards(ctx, ownerID, deckID, "name", false)
	if err != nil {
		return nil, fmt.Errorf("loading deck cards: %w", err)
	}
	pending, err := s.decks.GetPendingCards(ctx, ownerID, deckID)
	if err != nil {
		return nil, fmt.Errorf("loading pending cards: %w", err)
	}

	type key struct {
		scryfallID string
		foil       bool
	}
	index := make(map[key]int)
	var entries []collectEntry
	for _, c := range cards {
		if !boards[boardOf(c.Board)] {
			continue
		}
		k := key{c.ScryfallID, c.Foil}
		if i, ok := index[k]; ok {
			entries[i].quantity++
			continue
		}
		index[k] = len(entries)
		entries = append(entries, collectEntry{quantity: 1, card: card.Card{
			Name: c.Name, ScryfallID: c.ScryfallID, SetCode: c.SetCode, CollectorNumber: c.CollectorNumber, Foil: c.Foil,
			ManaValue: c.ManaValue, Colors: c.Colors, CardType: c.CardType, ColorIdentity: c.ColorIdentity,
		}})
	}
	for _, p := range pending {
		if !boards[boardOf(p.Board)] {
			continue
		}
		entries = append(entries, collectEntry{pendingID: p.ID, quantity: p.Quantity, card: card.Card{
			Name: p.Name, ScryfallID: p.ScryfallID, SetCode: p.SetCode, CollectorNumber: p.CollectorNumber, Foil: p.Foil,
			ManaValue: p.ManaValue, Colors: p.Colors, CardType: p.CardType, ColorIdentity: p.ColorIdentity,
		}})
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

	entries, err := s.collectEntries(ctx, source.ownerID, source.deck.ID, boards)
	if err != nil {
		return Summary{}, err
	}

	if req.Mode == CollectMissing {
		entries, err = s.withoutOwnedCopies(ctx, userID, entries)
		if err != nil {
			return Summary{}, err
		}
	}

	var summary Summary
	for _, e := range entries {
		if source.mine && e.pendingID != 0 {
			committed, err := s.CommitPendingCards(ctx, userID, source.deck.ID, req.StorageID, &e.pendingID, nil)
			summary.CardsCreated += committed.CardsCreated
			summary.CardsLinked += committed.CardsCreated
			if err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("%q: %v", e.card.Name, err))
			}
			continue
		}
		if req.Mode == CollectPending {
			continue
		}
		for i := 0; i < e.quantity; i++ {
			c := e.card
			c.StorageID = req.StorageID
			if _, err := s.cards.CreateCard(ctx, userID, c); err != nil {
				summary.CardsSkipped++
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not create %q: %v", c.Name, err))
				continue
			}
			summary.CardsCreated++
		}
	}
	return summary, nil
}

func (s *Service) withoutOwnedCopies(ctx context.Context, userID string, entries []collectEntry) ([]collectEntry, error) {
	keys := make([]string, 0, len(entries))
	seen := make(map[string]bool)
	for _, e := range entries {
		key := deck.CardNameKey(e.card.Name)
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	owned, err := s.decks.CountCopiesByName(ctx, userID, keys)
	if err != nil {
		return nil, fmt.Errorf("counting owned copies: %w", err)
	}

	missing := make([]collectEntry, 0, len(entries))
	for _, e := range entries {
		key := deck.CardNameKey(e.card.Name)
		covered := owned[key]
		if covered > e.quantity {
			covered = e.quantity
		}
		owned[key] -= covered
		if e.quantity > covered {
			e.quantity -= covered
			e.pendingID = 0
			missing = append(missing, e)
		}
	}
	return missing, nil
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

	result, err := h.service.DuplicateDeck(ctx.Request.Context(), userID, deckID)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
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
