package bulk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

var ErrCardsNotInDeck = errors.New("card_ids and pending_ids must list owned copies and pending cards of this deck")

type SwitchFoilRequest struct {
	CardIDs    []int `json:"card_ids"`
	PendingIDs []int `json:"pending_ids"`
	Foil       *bool `json:"foil"`
}

func (o *ownedCopies) takeFinish(scryfallID string, foil bool, proxy bool) (int, bool) {
	var candidates []card.Card
	for _, c := range o.byPrinting[strings.ToLower(scryfallID)] {
		if c.Foil == foil && c.Proxy == proxy && !o.used[c.ID] {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if o.inDeck[a.ID] != o.inDeck[b.ID] {
			return !o.inDeck[a.ID]
		}
		return a.ID < b.ID
	})
	o.used[candidates[0].ID] = true
	return candidates[0].ID, true
}

type FoilSwitch struct {
	CardIDs    []int
	PendingIDs []int
	Foil       bool
}

func (s *Service) SwitchFoil(ctx context.Context, userID string, deckID string, req FoilSwitch) (Summary, error) {
	return s.switchCards(ctx, userID, deckID, req.CardIDs, req.PendingIDs, func(p deck.PendingCard) deck.PendingCard {
		p.Foil = req.Foil
		return p
	})
}

func (s *Service) switchCards(ctx context.Context, userID string, deckID string, cardIDs []int, pendingIDs []int, apply func(deck.PendingCard) deck.PendingCard) (Summary, error) {
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return Summary{}, ErrDeckNotFound
		}
		return Summary{}, err
	}
	copies, err := s.deckCopies(ctx, userID, deckID, cardIDs)
	if err != nil {
		return Summary{}, err
	}
	pending, err := s.deckPending(ctx, userID, deckID, pendingIDs)
	if err != nil {
		return Summary{}, err
	}

	var switchedCopies []deck.DeckCard
	var copyTargets []deck.PendingCard
	for _, c := range copies {
		from := copyTemplate(c)
		if to := apply(from); !sameFinish(from, to) {
			switchedCopies = append(switchedCopies, c)
			copyTargets = append(copyTargets, to)
		}
	}
	var switchedPending []deck.PendingCard
	var pendingTargets []deck.PendingCard
	for _, p := range pending {
		from := p
		from.ID = 0
		from.Board = boardOf(p.Board)
		if to := apply(from); !sameFinish(from, to) {
			switchedPending = append(switchedPending, p)
			pendingTargets = append(pendingTargets, to)
		}
	}
	if len(switchedCopies) == 0 && len(switchedPending) == 0 {
		return Summary{}, nil
	}

	owned, err := s.loadOwnedCopies(ctx, userID, deckID, false)
	if err != nil {
		return Summary{}, err
	}

	var summary Summary
	for i, c := range switchedCopies {
		if err := s.replaceWithFinish(ctx, userID, deckID, owned, copyTargets[i], commanderSlot(d.CardSlot(c.ID)), &summary); err != nil {
			return summary, err
		}
		if err := s.decks.RemoveCardFromDeck(ctx, userID, deckID, c.ID); err != nil {
			return summary, fmt.Errorf("removing %q from the deck: %w", c.Name, err)
		}
		summary.CardsRemoved++
	}
	for i, p := range switchedPending {
		if err := s.replaceWithFinish(ctx, userID, deckID, owned, pendingTargets[i], commanderSlot(d.PendingSlot(p.ID)), &summary); err != nil {
			return summary, err
		}
		if err := s.decks.RemovePendingCard(ctx, userID, deckID, p.ID); err != nil {
			return summary, fmt.Errorf("removing the pending %q: %w", p.Name, err)
		}
		summary.CardsRemoved += p.Quantity
	}
	return summary, nil
}

func copyTemplate(c deck.DeckCard) deck.PendingCard {
	return deck.PendingCard{
		Name: c.Name, ScryfallID: c.ScryfallID, SetCode: c.SetCode, CollectorNumber: c.CollectorNumber,
		Foil: c.Foil, Proxy: c.Proxy,
		ManaValue: c.ManaValue, Colors: c.Colors, CardType: c.CardType, ColorIdentity: c.ColorIdentity,
		Quantity: 1, Board: boardOf(c.Board),
	}
}

func sameFinish(a deck.PendingCard, b deck.PendingCard) bool {
	return a.Foil == b.Foil && a.Proxy == b.Proxy
}

func (s *Service) deckCopies(ctx context.Context, userID string, deckID string, ids []int) ([]deck.DeckCard, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	deckCards, err := s.decks.GetDeckCards(ctx, userID, deckID, "updated", true)
	if err != nil {
		return nil, fmt.Errorf("loading deck cards: %w", err)
	}
	byID := make(map[int]deck.DeckCard, len(deckCards))
	for _, c := range deckCards {
		byID[c.ID] = c
	}
	seen := make(map[int]bool, len(ids))
	var copies []deck.DeckCard
	for _, id := range ids {
		c, ok := byID[id]
		if !ok {
			return nil, ErrCardsNotInDeck
		}
		if !seen[id] {
			copies = append(copies, c)
		}
		seen[id] = true
	}
	return copies, nil
}

func (s *Service) deckPending(ctx context.Context, userID string, deckID string, ids []int) ([]deck.PendingCard, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	items, err := s.decks.GetPendingCards(ctx, userID, deckID)
	if err != nil {
		return nil, fmt.Errorf("loading pending cards: %w", err)
	}
	byID := make(map[int]deck.PendingCard, len(items))
	for _, p := range items {
		byID[p.ID] = p
	}
	seen := make(map[int]bool, len(ids))
	var pending []deck.PendingCard
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			return nil, ErrCardsNotInDeck
		}
		if !seen[id] {
			pending = append(pending, p)
		}
		seen[id] = true
	}
	return pending, nil
}

func commanderSlot(slot deck.CommanderSlot, ok bool) *deck.CommanderSlot {
	if !ok {
		return nil
	}
	return &slot
}

func (s *Service) replaceWithFinish(ctx context.Context, userID string, deckID string, owned *ownedCopies, want deck.PendingCard, commander *deck.CommanderSlot, summary *Summary) error {
	firstCopy := 0
	linked := 0
	for linked < want.Quantity {
		copyID, ok := owned.takeFinish(want.ScryfallID, want.Foil, want.Proxy)
		if !ok {
			break
		}
		if err := s.decks.PutCardInDeck(ctx, userID, deckID, copyID, want.Board); err != nil {
			return fmt.Errorf("linking the %s copy of %q: %w", finishName(want), want.Name, err)
		}
		if linked == 0 {
			firstCopy = copyID
		}
		linked++
	}
	summary.CardsLinked += linked

	var added deck.PendingCard
	if remaining := want.Quantity - linked; remaining > 0 {
		want.DeckID = deckID
		want.Quantity = remaining
		var err error
		if added, err = s.decks.AddPendingCard(ctx, userID, deckID, want); err != nil {
			return fmt.Errorf("adding a pending %s %q: %w", finishName(want), want.Name, err)
		}
		summary.CardsPending += remaining
	}

	if commander == nil {
		return nil
	}
	if firstCopy != 0 {
		if err := s.decks.SetCardCommander(ctx, userID, deckID, *commander, firstCopy); err != nil {
			return fmt.Errorf("moving the commander: %w", err)
		}
		return nil
	}
	if err := s.decks.SetPendingCommander(ctx, userID, deckID, *commander, added.ID); err != nil {
		return fmt.Errorf("moving the commander: %w", err)
	}
	return nil
}

func finishName(c deck.PendingCard) string {
	name := "non-foil"
	if c.Foil {
		name = "foil"
	}
	if c.Proxy {
		name += " proxy"
	}
	return name
}

func (h *Handler) switchFoil(ctx *gin.Context) {
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
	var req SwitchFoilRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil || req.Foil == nil || len(req.CardIDs)+len(req.PendingIDs) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "foil and at least one of card_ids or pending_ids are required"})
		return
	}

	summary, err := h.service.SwitchFoil(ctx.Request.Context(), userID, deckID, FoilSwitch{CardIDs: req.CardIDs, PendingIDs: req.PendingIDs, Foil: *req.Foil})
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrCardsNotInDeck, Status: http.StatusBadRequest},
		)
		return
	}
	ctx.JSON(http.StatusOK, summary)
}
