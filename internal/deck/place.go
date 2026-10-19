package deck

import (
	"context"
	"fmt"
	"strings"
)

// Printing is a card printing with the Scryfall details stored on its copies
// and pending cards.
type Printing struct {
	Name            string
	ScryfallID      string
	SetCode         string
	CollectorNumber string
	ManaValue       float64
	Colors          *string
	CardType        *string
	ColorIdentity   *string
}

// PlacementPlan is a change PlaceCards applies to a deck in one transaction.
// Tags are added to those the deck's cards already have, by card name.
type PlacementPlan struct {
	Lines                 []PlacementLine
	Tags                  []TaggedCard
	IncludeLockedStorages bool
}

// PlacementLine asks for Quantity copies of a printing on a board. Owned
// copies are taken from the printing, or from any printing of the card when
// AnyPrinting is set; pending cards always get the printing. The first two
// Commander lines become the commander then the second commander, unless the
// deck already has a commander.
type PlacementLine struct {
	Printing    Printing
	AnyPrinting bool
	Foil        bool
	Proxy       ProxyMatch
	Board       string
	Quantity    int
	Commander   bool
}

// ProxyMatch says which owned copies a line accepts. Pending cards added for
// AnyProxy lines are real cards.
type ProxyMatch int

const (
	AnyProxy ProxyMatch = iota
	RealOnly
	ProxyOnly
)

func (m ProxyMatch) accepts(proxy bool) bool {
	return m == AnyProxy || (m == ProxyOnly) == proxy
}

type PlacementSummary struct {
	CardsLinked  int
	CardsPending int
	SkippedTags  []SkippedTags
}

// SkippedTags are tags of the plan left out without failing the placement:
// the card isn't in the deck (ErrCardNotInDeck) or the tags are invalid.
type SkippedTags struct {
	CardName string
	Err      error
}

// PlaceCards puts the plan's cards in the deck: free owned copies first, then
// pending cards for the rest.
func (s *Service) PlaceCards(ctx context.Context, userID string, deckID string, plan PlacementPlan) (PlacementSummary, error) {
	var summary PlacementSummary
	err := s.repo.Compose(ctx, userID, deckID, func(store CompositionStore) error {
		summary = PlacementSummary{}
		p, err := newPlacement(ctx, store, userID, deckID, plan)
		if err != nil {
			return err
		}
		for _, line := range plan.Lines {
			if err := p.placeLine(ctx, line, &summary); err != nil {
				return err
			}
		}
		if err := p.saveCommanders(ctx); err != nil {
			return err
		}
		summary.SkippedTags, err = p.addTags(ctx, plan.Tags)
		return err
	})
	if err != nil {
		return PlacementSummary{}, err
	}
	return summary, nil
}

type placement struct {
	store   CompositionStore
	userID  string
	deck    Deck
	copies  *copyPicker
	pending []PendingCard

	nextSlot        CommanderSlot
	commandersMoved bool
}

func newPlacement(ctx context.Context, store CompositionStore, userID string, deckID string, plan PlacementPlan) (*placement, error) {
	d, err := store.FindByID(ctx, userID, deckID)
	if err != nil {
		return nil, err
	}
	pending, err := store.FindPendingCards(ctx, userID, deckID)
	if err != nil {
		return nil, err
	}
	var wanted CopiesWanted
	for _, line := range plan.Lines {
		if line.AnyPrinting {
			wanted.Names = append(wanted.Names, line.Printing.Name)
		} else {
			wanted.ScryfallIDs = append(wanted.ScryfallIDs, line.Printing.ScryfallID)
		}
	}
	copies, err := findOwnedCopies(ctx, store, userID, deckID, wanted)
	if err != nil {
		return nil, err
	}
	p := &placement{store: store, userID: userID, deck: d, copies: newCopyPicker(copies, plan.IncludeLockedStorages), pending: pending, nextSlot: SlotCommander}
	if d.HasCommander() {
		p.nextSlot = SlotNone
	}
	return p, nil
}

func (p *placement) placeLine(ctx context.Context, line PlacementLine, summary *PlacementSummary) error {
	board := line.Board
	if board == "" {
		board = BoardMain
	}
	commander := line.Commander && p.nextSlot != SlotNone
	if commander && !InMainBoard(board) {
		return ErrCommanderBoard
	}
	ids := p.copies.take(line)
	for _, id := range ids {
		if err := p.store.LinkCardToDeck(ctx, p.userID, p.deck.ID, id, board); err != nil {
			return fmt.Errorf("putting %q in the deck: %w", line.Printing.Name, err)
		}
	}
	summary.CardsLinked += len(ids)
	if commander && len(ids) > 0 {
		p.fillSlot(&ids[0], nil)
	}
	missing := line.Quantity - len(ids)
	if missing <= 0 {
		return nil
	}
	pending, err := p.addPending(ctx, line, board, missing)
	if err != nil {
		return fmt.Errorf("adding %q to the deck: %w", line.Printing.Name, err)
	}
	summary.CardsPending += missing
	if commander && len(ids) == 0 {
		p.fillSlot(nil, &pending.ID)
	}
	return nil
}

func (p *placement) fillSlot(cardID, pendingID *int) {
	p.deck.setSlot(p.nextSlot, cardID, pendingID)
	p.commandersMoved = true
	p.nextSlot++
	if p.nextSlot > SlotPartner {
		p.nextSlot = SlotNone
	}
}

func (p *placement) saveCommanders(ctx context.Context) error {
	if !p.commandersMoved {
		return nil
	}
	if err := p.deck.checkCommanders(); err != nil {
		return err
	}
	_, err := p.store.Update(ctx, p.userID, p.deck)
	return err
}

func (p *placement) addPending(ctx context.Context, line PlacementLine, board string, quantity int) (PendingCard, error) {
	card := PendingCard{
		DeckID:          p.deck.ID,
		Name:            line.Printing.Name,
		ScryfallID:      line.Printing.ScryfallID,
		SetCode:         line.Printing.SetCode,
		CollectorNumber: line.Printing.CollectorNumber,
		Foil:            line.Foil,
		Proxy:           line.Proxy == ProxyOnly,
		Quantity:        quantity,
		ManaValue:       line.Printing.ManaValue,
		Colors:          line.Printing.Colors,
		CardType:        line.Printing.CardType,
		ColorIdentity:   line.Printing.ColorIdentity,
		Board:           board,
	}
	if e, ok := mergeTarget(p.deck, p.pending, card, 0); ok {
		merged, err := p.store.UpdatePendingQuantity(ctx, p.userID, p.deck.ID, e.ID, e.Quantity+quantity)
		if err != nil {
			return PendingCard{}, err
		}
		p.replacePending(merged)
		return merged, nil
	}
	created, err := p.store.CreatePendingCard(ctx, p.userID, card)
	if err != nil {
		return PendingCard{}, err
	}
	p.pending = append(p.pending, created)
	return created, nil
}

func (p *placement) replacePending(updated PendingCard) {
	for i := range p.pending {
		if p.pending[i].ID == updated.ID {
			p.pending[i] = updated
		}
	}
}

func (p *placement) addTags(ctx context.Context, tagged []TaggedCard) ([]SkippedTags, error) {
	if len(tagged) == 0 {
		return nil, nil
	}
	owned, err := p.store.FindCardsByDeckID(ctx, p.userID, p.deck.ID, "added", false)
	if err != nil {
		return nil, err
	}
	names := cardNames(mergeDeckEntries(p.deck, owned, p.pending))
	existing, err := existingTags(ctx, p.store, p.userID, p.deck.ID)
	if err != nil {
		return nil, err
	}
	rows, err := p.store.FindCardTags(ctx, p.userID, p.deck.ID)
	if err != nil {
		return nil, err
	}
	current := make(map[string][]string)
	for _, row := range rows {
		current[row.CardName] = append(current[row.CardName], row.Tag)
	}

	var skipped []SkippedTags
	for _, card := range tagged {
		key := CardNameKey(card.Name)
		if _, inDeck := names[key]; !inDeck {
			skipped = append(skipped, SkippedTags{CardName: card.Name, Err: ErrCardNotInDeck})
			continue
		}
		tags, err := cleanCardTags(existing, append(append([]string{}, current[key]...), card.Tags...))
		if err != nil {
			skipped = append(skipped, SkippedTags{CardName: card.Name, Err: err})
			continue
		}
		for _, tag := range tags {
			existing[strings.ToLower(tag)] = tag
		}
		current[key] = tags
		if err := p.store.WriteCardTags(ctx, p.userID, p.deck.ID, key, tags); err != nil {
			return nil, fmt.Errorf("tagging %q: %w", card.Name, err)
		}
	}
	return skipped, nil
}
