package deck

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrSplitMismatch = errors.New("the copies to split must be one card on one board, outside the commander, with one printing per copy")

type PrintingChoice struct {
	ScryfallID      string
	SetCode         string
	CollectorNumber string
	Foil            bool
}

// SplitPrintings gives each selected copy (owned copies, plus each pending
// card's quantity) one of printings. Copies that already have a wanted
// printing and finish stay; the others leave the deck and the remaining
// printings are placed on the same board as SwitchFinish places them.
func (s *Service) SplitPrintings(ctx context.Context, userID string, deckID string, cards CardSelection, printings []PrintingChoice) (SwitchSummary, error) {
	return s.compose(ctx, userID, deckID, func(c *composition) error {
		copies, pending, err := c.selected(cards)
		if err != nil {
			return err
		}
		template, err := splitTemplate(c.deck, copies, pending, len(printings))
		if err != nil {
			return err
		}

		keptCopies := make(map[int]bool, len(copies))
		keptPending := make(map[int]int, len(pending))
		var toPlace []PrintingChoice
		for _, want := range printings {
			if keepMatchingCopy(copies, keptCopies, want) || keepMatchingPending(pending, keptPending, want) {
				continue
			}
			toPlace = append(toPlace, want)
		}
		if len(toPlace) == 0 {
			return errNothingToSwitch
		}

		if err := c.removeUnkept(copies, keptCopies, pending, keptPending); err != nil {
			return err
		}
		targets := make([]PendingCard, 0, len(toPlace))
		for _, group := range groupPrintings(toPlace) {
			want := template
			want.ScryfallID = group.choice.ScryfallID
			want.SetCode = group.choice.SetCode
			want.CollectorNumber = group.choice.CollectorNumber
			want.Foil = group.choice.Foil
			want.Quantity = group.quantity
			targets = append(targets, want)
		}
		if err := c.loadOwnedCopies(targets); err != nil {
			return err
		}
		for _, want := range targets {
			if err := c.place(want, SlotNone, false); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *composition) removeUnkept(copies []DeckCard, keptCopies map[int]bool, pending []PendingCard, keptPending map[int]int) error {
	for _, dc := range copies {
		if keptCopies[dc.ID] {
			continue
		}
		if err := c.store.UnlinkCardFromDeck(c.ctx, c.userID, c.deck.ID, dc.ID); err != nil {
			return fmt.Errorf("removing %q from the deck: %w", dc.Name, err)
		}
		c.summary.CardsRemoved++
	}
	for _, p := range pending {
		kept := keptPending[p.ID]
		if kept == p.Quantity {
			continue
		}
		var err error
		if kept == 0 {
			err = c.store.DeletePendingCard(c.ctx, c.userID, c.deck.ID, p.ID)
		} else {
			_, err = c.store.UpdatePendingQuantity(c.ctx, c.userID, c.deck.ID, p.ID, kept)
		}
		if err != nil {
			return fmt.Errorf("updating the pending %q: %w", p.Name, err)
		}
		c.summary.CardsRemoved += p.Quantity - kept
	}
	return nil
}

func splitTemplate(d Deck, copies []DeckCard, pending []PendingCard, printings int) (PendingCard, error) {
	var sources []PendingCard
	units := 0
	for _, dc := range copies {
		if d.IsCommanderCard(dc.ID) {
			return PendingCard{}, ErrSplitMismatch
		}
		sources = append(sources, copyTemplate(dc))
		units++
	}
	for _, p := range pending {
		if d.IsCommanderPending(p.ID) {
			return PendingCard{}, ErrSplitMismatch
		}
		sources = append(sources, pendingTemplate(p))
		units += p.Quantity
	}
	if len(sources) == 0 || units != printings {
		return PendingCard{}, ErrSplitMismatch
	}
	template := sources[0]
	for _, src := range sources[1:] {
		if CardNameKey(src.Name) != CardNameKey(template.Name) || src.Board != template.Board {
			return PendingCard{}, ErrSplitMismatch
		}
	}
	template.Proxy = false
	return template, nil
}

func keepMatchingCopy(copies []DeckCard, kept map[int]bool, want PrintingChoice) bool {
	for _, dc := range copies {
		if !kept[dc.ID] && dc.Foil == want.Foil && strings.EqualFold(dc.ScryfallID, want.ScryfallID) {
			kept[dc.ID] = true
			return true
		}
	}
	return false
}

func keepMatchingPending(pending []PendingCard, kept map[int]int, want PrintingChoice) bool {
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
	for _, ch := range choices {
		key := fmt.Sprintf("%s|%t", strings.ToLower(ch.ScryfallID), ch.Foil)
		if i, ok := index[key]; ok {
			groups[i].quantity++
			continue
		}
		index[key] = len(groups)
		groups = append(groups, printingGroup{choice: ch, quantity: 1})
	}
	return groups
}
