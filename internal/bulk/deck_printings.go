package bulk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/deck"
)

var ErrSplitMismatch = errors.New("the copies to split must be one card on one board, outside the commander, with one printing per copy")

type SplitPrintingsRequest struct {
	CardIDs    []int            `json:"card_ids"`
	PendingIDs []int            `json:"pending_ids"`
	Printings  []PrintingChoice `json:"printings"`
}

type PrintingChoice struct {
	ScryfallID      string `json:"scryfall_id"`
	SetCode         string `json:"set_code"`
	CollectorNumber string `json:"collector_number"`
	Foil            bool   `json:"foil"`
}

type PrintingSplit struct {
	CardIDs    []int
	PendingIDs []int
	Printings  []PrintingChoice
}

func (s *Service) SplitPrintings(ctx context.Context, userID string, deckID string, req PrintingSplit) (Summary, error) {
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return Summary{}, ErrDeckNotFound
		}
		return Summary{}, err
	}
	copies, err := s.deckCopies(ctx, userID, deckID, req.CardIDs)
	if err != nil {
		return Summary{}, err
	}
	pending, err := s.deckPending(ctx, userID, deckID, req.PendingIDs)
	if err != nil {
		return Summary{}, err
	}
	template, err := splitTemplate(d, copies, pending, len(req.Printings))
	if err != nil {
		return Summary{}, err
	}

	keptCopies := make(map[int]bool, len(copies))
	keptPending := make(map[int]int, len(pending))
	var toPlace []PrintingChoice
	for _, want := range req.Printings {
		if keepMatchingCopy(copies, keptCopies, want) || keepMatchingPending(pending, keptPending, want) {
			continue
		}
		toPlace = append(toPlace, want)
	}
	if len(toPlace) == 0 {
		return Summary{}, nil
	}

	owned, err := s.loadOwnedCopies(ctx, userID, deckID, false)
	if err != nil {
		return Summary{}, err
	}

	var summary Summary
	for _, c := range copies {
		if keptCopies[c.ID] {
			continue
		}
		if err := s.decks.RemoveCardFromDeck(ctx, userID, deckID, c.ID); err != nil {
			return summary, fmt.Errorf("removing %q from the deck: %w", c.Name, err)
		}
		summary.CardsRemoved++
	}
	for _, p := range pending {
		kept := keptPending[p.ID]
		if kept == p.Quantity {
			continue
		}
		if kept == 0 {
			if err := s.decks.RemovePendingCard(ctx, userID, deckID, p.ID); err != nil {
				return summary, fmt.Errorf("removing the pending %q: %w", p.Name, err)
			}
		} else if _, err := s.decks.SetPendingQuantity(ctx, userID, deckID, p.ID, kept); err != nil {
			return summary, fmt.Errorf("updating the pending %q: %w", p.Name, err)
		}
		summary.CardsRemoved += p.Quantity - kept
	}

	for _, group := range groupPrintings(toPlace) {
		want := template
		want.ScryfallID = group.choice.ScryfallID
		want.SetCode = group.choice.SetCode
		want.CollectorNumber = group.choice.CollectorNumber
		want.Foil = group.choice.Foil
		want.Quantity = group.quantity
		if err := s.replaceWithFinish(ctx, userID, deckID, owned, want, nil, &summary); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func splitTemplate(d deck.Deck, copies []deck.DeckCard, pending []deck.PendingCard, printings int) (deck.PendingCard, error) {
	var sources []deck.PendingCard
	units := 0
	for _, c := range copies {
		if d.IsCommanderCard(c.ID) {
			return deck.PendingCard{}, ErrSplitMismatch
		}
		sources = append(sources, deck.PendingCard{
			Name: c.Name, ManaValue: c.ManaValue, Colors: c.Colors, CardType: c.CardType, ColorIdentity: c.ColorIdentity,
			Board: boardOf(c.Board),
		})
		units++
	}
	for _, p := range pending {
		if d.IsCommanderPending(p.ID) {
			return deck.PendingCard{}, ErrSplitMismatch
		}
		sources = append(sources, deck.PendingCard{
			Name: p.Name, ManaValue: p.ManaValue, Colors: p.Colors, CardType: p.CardType, ColorIdentity: p.ColorIdentity,
			Board: boardOf(p.Board),
		})
		units += p.Quantity
	}
	if len(sources) == 0 || units != printings {
		return deck.PendingCard{}, ErrSplitMismatch
	}
	template := sources[0]
	for _, src := range sources[1:] {
		if deck.CardNameKey(src.Name) != deck.CardNameKey(template.Name) || src.Board != template.Board {
			return deck.PendingCard{}, ErrSplitMismatch
		}
	}
	return template, nil
}

func keepMatchingCopy(copies []deck.DeckCard, kept map[int]bool, want PrintingChoice) bool {
	for _, c := range copies {
		if !kept[c.ID] && c.Foil == want.Foil && strings.EqualFold(c.ScryfallID, want.ScryfallID) {
			kept[c.ID] = true
			return true
		}
	}
	return false
}

func keepMatchingPending(pending []deck.PendingCard, kept map[int]int, want PrintingChoice) bool {
	for _, p := range pending {
		if kept[p.ID] < p.Quantity && p.Foil == want.Foil && strings.EqualFold(p.ScryfallID, want.ScryfallID) {
			kept[p.ID]++
			return true
		}
	}
	return false
}

type printingGroup struct {
	choice   PrintingChoice
	quantity int
}

func groupPrintings(choices []PrintingChoice) []printingGroup {
	var groups []printingGroup
	index := make(map[string]int)
	for _, c := range choices {
		key := fmt.Sprintf("%s|%t", strings.ToLower(c.ScryfallID), c.Foil)
		if i, ok := index[key]; ok {
			groups[i].quantity++
			continue
		}
		index[key] = len(groups)
		groups = append(groups, printingGroup{choice: c, quantity: 1})
	}
	return groups
}

func (h *Handler) splitPrintings(ctx *gin.Context) {
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
	var req SplitPrintingsRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil || len(req.CardIDs)+len(req.PendingIDs) == 0 || !validPrintings(req.Printings) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "printings, each with a scryfall_id, and at least one of card_ids or pending_ids are required"})
		return
	}

	summary, err := h.service.SplitPrintings(ctx.Request.Context(), userID, deckID, PrintingSplit(req))
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrCardsNotInDeck, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrSplitMismatch, Status: http.StatusBadRequest},
		)
		return
	}
	ctx.JSON(http.StatusOK, summary)
}

func validPrintings(printings []PrintingChoice) bool {
	if len(printings) == 0 {
		return false
	}
	for _, p := range printings {
		if strings.TrimSpace(p.ScryfallID) == "" {
			return false
		}
	}
	return true
}
