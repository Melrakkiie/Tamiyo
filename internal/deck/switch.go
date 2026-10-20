package deck

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrCardsNotInDeck = errors.New("card_ids and pending_ids must list owned copies and pending cards of this deck")

// errNothingToSwitch rolls back a composition that changes nothing, so the
// deck's updated date stays as it was.
var errNothingToSwitch = errors.New("nothing to switch")

// CardSelection lists owned copies (CardIDs) and pending cards (PendingIDs)
// of a deck.
type CardSelection struct {
	CardIDs    []int
	PendingIDs []int
}

// SwitchSummary counts the copies an operation put in the deck from the
// collection, the copies it left pending and the copies that left the deck.
type SwitchSummary struct {
	CardsLinked  int
	CardsPending int
	CardsRemoved int
}

// SwitchFinish replaces the selected copies and pending cards not already in
// that finish by copies of the same printing and proxy state in that finish
// (ADR-0005), on the same board and in the same commander slot.
func (s *Service) SwitchFinish(ctx context.Context, userID string, deckID string, cards CardSelection, foil bool) (SwitchSummary, error) {
	return s.switchCards(ctx, userID, deckID, cards, func(p PendingCard) PendingCard {
		p.Foil = foil
		return p
	})
}

// SwitchProxy works like SwitchFinish for the proxy state: each card keeps
// its finish.
func (s *Service) SwitchProxy(ctx context.Context, userID string, deckID string, cards CardSelection, proxy bool) (SwitchSummary, error) {
	return s.switchCards(ctx, userID, deckID, cards, func(p PendingCard) PendingCard {
		p.Proxy = proxy
		return p
	})
}

type switchedCard struct {
	cardID  int
	pending *PendingCard
	target  PendingCard
	slot    CommanderSlot
	hasSlot bool
	removed int
}

func (s *Service) switchCards(ctx context.Context, userID string, deckID string, cards CardSelection, change func(PendingCard) PendingCard) (SwitchSummary, error) {
	return s.compose(ctx, userID, deckID, func(c *composition) error {
		copies, pending, err := c.selected(cards)
		if err != nil {
			return err
		}
		var switched []switchedCard
		for _, dc := range copies {
			from := copyTemplate(dc)
			if to := change(from); !sameFinish(from, to) {
				slot, ok := c.deck.CardSlot(dc.ID)
				switched = append(switched, switchedCard{cardID: dc.ID, target: to, slot: slot, hasSlot: ok, removed: 1})
			}
		}
		for _, p := range pending {
			from := pendingTemplate(p)
			if to := change(from); !sameFinish(from, to) {
				slot, ok := c.deck.PendingSlot(p.ID)
				switched = append(switched, switchedCard{pending: &p, target: to, slot: slot, hasSlot: ok, removed: p.Quantity})
			}
		}
		if len(switched) == 0 {
			return errNothingToSwitch
		}

		targets := make([]PendingCard, 0, len(switched))
		for _, sw := range switched {
			targets = append(targets, sw.target)
		}
		if err := c.loadOwnedCopies(targets); err != nil {
			return err
		}
		for _, sw := range switched {
			if err := c.place(sw.target, sw.slot, sw.hasSlot); err != nil {
				return err
			}
			if sw.pending != nil {
				err = c.store.DeletePendingCard(ctx, userID, deckID, sw.pending.ID)
			} else {
				err = c.store.UnlinkCardFromDeck(ctx, userID, deckID, sw.cardID)
			}
			if err != nil {
				return fmt.Errorf("removing %q from the deck: %w", sw.target.Name, err)
			}
			c.summary.CardsRemoved += sw.removed
		}
		return nil
	})
}

// composition is one switch or split in progress, inside the transaction.
type composition struct {
	ctx     context.Context
	store   CompositionStore
	userID  string
	deck    Deck
	owned   []OwnedCopy
	used    map[int]bool
	summary SwitchSummary
}

func (s *Service) compose(ctx context.Context, userID string, deckID string, apply func(*composition) error) (SwitchSummary, error) {
	var summary SwitchSummary
	err := s.repo.Compose(ctx, userID, deckID, func(store CompositionStore) error {
		d, err := store.FindByID(ctx, userID, deckID)
		if err != nil {
			return err
		}
		c := &composition{ctx: ctx, store: store, userID: userID, deck: d, used: make(map[int]bool)}
		if err := apply(c); err != nil {
			return err
		}
		summary = c.summary
		return nil
	})
	if errors.Is(err, errNothingToSwitch) {
		return SwitchSummary{}, nil
	}
	if err != nil {
		return SwitchSummary{}, err
	}
	return summary, nil
}

func (c *composition) selected(cards CardSelection) ([]DeckCard, []PendingCard, error) {
	var copies []DeckCard
	if len(cards.CardIDs) > 0 {
		deckCards, err := c.store.FindCardsByDeckID(c.ctx, c.userID, c.deck.ID, "added", false)
		if err != nil {
			return nil, nil, err
		}
		if copies, err = pick(deckCards, cards.CardIDs, func(dc DeckCard) int { return dc.ID }); err != nil {
			return nil, nil, err
		}
	}
	var pending []PendingCard
	if len(cards.PendingIDs) > 0 {
		all, err := c.store.FindPendingCards(c.ctx, c.userID, c.deck.ID)
		if err != nil {
			return nil, nil, err
		}
		if pending, err = pick(all, cards.PendingIDs, func(p PendingCard) int { return p.ID }); err != nil {
			return nil, nil, err
		}
	}
	return copies, pending, nil
}

func pick[T any](items []T, ids []int, id func(T) int) ([]T, error) {
	byID := make(map[int]T, len(items))
	for _, item := range items {
		byID[id(item)] = item
	}
	seen := make(map[int]bool, len(ids))
	picked := make([]T, 0, len(ids))
	for _, wanted := range ids {
		item, ok := byID[wanted]
		if !ok {
			return nil, ErrCardsNotInDeck
		}
		if !seen[wanted] {
			picked = append(picked, item)
		}
		seen[wanted] = true
	}
	return picked, nil
}

func (c *composition) loadOwnedCopies(targets []PendingCard) error {
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, strings.ToLower(t.ScryfallID))
	}
	owned, err := c.store.FindOwnedCopies(c.ctx, c.userID, c.deck.ID, ids, nil)
	if err != nil {
		return fmt.Errorf("finding owned copies: %w", err)
	}
	c.owned = owned
	return nil
}

// takeCopy picks an owned copy of the printing in that finish and proxy
// state, outside the deck and storages locked against it: one in no deck
// first, then one another deck uses (ADR-0005).
func (c *composition) takeCopy(want PendingCard) (int, bool) {
	best := -1
	for i, o := range c.owned {
		if c.used[o.ID] || o.InTargetDeck || o.LockedAgainstTarget || o.Foil != want.Foil || o.Proxy != want.Proxy || !strings.EqualFold(o.ScryfallID, want.ScryfallID) {
			continue
		}
		if best == -1 || c.owned[best].InDeck && !o.InDeck {
			best = i
		}
	}
	if best == -1 {
		return 0, false
	}
	c.used[c.owned[best].ID] = true
	return c.owned[best].ID, true
}

// place puts want.Quantity copies of want in the deck on want.Board: owned
// copies first, a pending card for the rest. A commander slot goes to the
// first owned copy, or else to the pending card.
func (c *composition) place(want PendingCard, slot CommanderSlot, hasSlot bool) error {
	firstCopy, linked := 0, 0
	for linked < want.Quantity {
		copyID, ok := c.takeCopy(want)
		if !ok {
			break
		}
		if err := c.store.LinkCardToDeck(c.ctx, c.userID, c.deck.ID, copyID, want.Board); err != nil {
			return fmt.Errorf("putting %q in the deck: %w", want.Name, err)
		}
		if linked == 0 {
			firstCopy = copyID
		}
		linked++
	}
	c.summary.CardsLinked += linked

	pendingID := 0
	if remaining := want.Quantity - linked; remaining > 0 {
		want.Quantity = remaining
		added, err := c.addPending(want, hasSlot)
		if err != nil {
			return fmt.Errorf("adding %q to the pending cards: %w", want.Name, err)
		}
		pendingID = added.ID
		c.summary.CardsPending += remaining
	}

	if !hasSlot {
		return nil
	}
	if firstCopy != 0 {
		c.deck.setSlot(slot, &firstCopy, nil)
	} else {
		c.deck.setSlot(slot, nil, &pendingID)
	}
	updated, err := c.store.Update(c.ctx, c.userID, c.deck)
	if err != nil {
		return fmt.Errorf("keeping %q in its commander slot: %w", want.Name, err)
	}
	c.deck = updated
	return nil
}

func (c *composition) addPending(p PendingCard, commander bool) (PendingCard, error) {
	existing, err := c.store.FindPendingCards(c.ctx, c.userID, c.deck.ID)
	if err != nil {
		return PendingCard{}, err
	}
	return addPendingCard(c.ctx, c.store, c.userID, c.deck, existing, p, commander)
}

func copyTemplate(dc DeckCard) PendingCard {
	return PendingCard{
		Name: dc.Name, ScryfallID: dc.ScryfallID, SetCode: dc.SetCode, CollectorNumber: dc.CollectorNumber,
		Foil: dc.Foil, Proxy: dc.Proxy,
		ManaValue: dc.ManaValue, Colors: dc.Colors, CardType: dc.CardType, ColorIdentity: dc.ColorIdentity,
		Quantity: 1, Board: BoardOrMain(dc.Board),
	}
}

func pendingTemplate(p PendingCard) PendingCard {
	return PendingCard{
		Name: p.Name, ScryfallID: p.ScryfallID, SetCode: p.SetCode, CollectorNumber: p.CollectorNumber,
		Foil: p.Foil, Proxy: p.Proxy,
		ManaValue: p.ManaValue, Colors: p.Colors, CardType: p.CardType, ColorIdentity: p.ColorIdentity,
		Quantity: p.Quantity, Board: BoardOrMain(p.Board),
	}
}

// BoardOrMain is the board a card with no board is on.
func BoardOrMain(board string) string {
	if board == "" {
		return BoardMain
	}
	return board
}

func sameFinish(a PendingCard, b PendingCard) bool {
	return a.Foil == b.Foil && a.Proxy == b.Proxy
}
