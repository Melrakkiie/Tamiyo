package bulk

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

type ownedCopies struct {
	byPrinting map[string][]card.Card
	inDeck     map[int]bool
	used       map[int]bool
}

func (o *ownedCopies) take(scryfallID string, foil bool, quantity int) []int {
	candidates := append([]card.Card(nil), o.byPrinting[strings.ToLower(scryfallID)]...)
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if o.inDeck[a.ID] != o.inDeck[b.ID] {
			return !o.inDeck[a.ID]
		}
		if (a.Foil == foil) != (b.Foil == foil) {
			return a.Foil == foil
		}
		return a.ID < b.ID
	})
	ids := make([]int, 0, quantity)
	for _, c := range candidates {
		if len(ids) == quantity {
			break
		}
		if o.used[c.ID] {
			continue
		}
		o.used[c.ID] = true
		ids = append(ids, c.ID)
	}
	return ids
}

func (s *Service) loadOwnedCopies(ctx context.Context, userID string) (*ownedCopies, error) {
	cards, err := s.loadAllCards(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loading the collection: %w", err)
	}
	owned := &ownedCopies{byPrinting: make(map[string][]card.Card), inDeck: make(map[int]bool), used: make(map[int]bool)}
	for _, c := range cards {
		key := strings.ToLower(c.ScryfallID)
		owned.byPrinting[key] = append(owned.byPrinting[key], c)
	}

	const limit = 100
	for page := 1; ; page++ {
		decks, total, err := s.decks.GetAllDecks(ctx, userID, deck.Filter{Page: page, Limit: limit})
		if err != nil {
			return nil, fmt.Errorf("loading decks: %w", err)
		}
		for _, d := range decks {
			deckCards, err := s.decks.GetDeckCards(ctx, userID, d.ID, "updated", true)
			if err != nil {
				return nil, fmt.Errorf("loading the cards of deck %q: %w", d.Name, err)
			}
			for _, c := range deckCards {
				owned.inDeck[c.ID] = true
			}
		}
		if len(decks) == 0 || page*limit >= total {
			return owned, nil
		}
	}
}

type deckLinePlacement struct {
	line      moxfieldDeckLine
	resolved  ResolvedCard
	ownedIDs  []int
	missing   int
	commander bool
}

func (s *Service) ImportMoxfieldDeck(ctx context.Context, userID string, req MoxfieldDeckImportRequest, r io.Reader) (Summary, error) {
	lines, err := parseMoxfieldDeckList(r)
	if err != nil {
		return Summary{}, err
	}

	identifiers := make([]CardIdentifier, len(lines))
	for i, line := range lines {
		identifiers[i] = CardIdentifier{SetCode: line.SetCode, CollectorNumber: line.CollectorNumber}
	}
	resolved, err := s.scryfall.Resolve(ctx, dedupeIdentifiers(identifiers))
	if err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
	}

	owned, err := s.loadOwnedCopies(ctx, userID)
	if err != nil {
		return Summary{}, err
	}

	var summary Summary
	placements := make([]deckLinePlacement, 0, len(lines))
	for i, line := range lines {
		commander := req.CommanderFromFirstLine && i == 0
		resolvedCard, ok := resolved[resolveKey(line.SetCode, line.CollectorNumber)]
		if !ok {
			summary.CardsSkipped += line.Quantity
			label := ""
			if commander {
				label = "commander "
			}
			summary.Warnings = append(summary.Warnings, fmt.Sprintf(
				"line %d: %s%q (%s #%s) not found on scryfall", line.LineNo, label, line.CardName, line.SetCode, line.CollectorNumber,
			))
			continue
		}
		ownedIDs := owned.take(resolvedCard.ScryfallID, line.Foil, line.Quantity)
		placements = append(placements, deckLinePlacement{
			line:      line,
			resolved:  resolvedCard,
			ownedIDs:  ownedIDs,
			missing:   line.Quantity - len(ownedIDs),
			commander: commander,
		})
	}

	var commanderID *int
	if len(placements) > 0 && placements[0].commander && len(placements[0].ownedIDs) > 0 {
		commanderID = &placements[0].ownedIDs[0]
	}

	createdDeck, err := s.decks.CreateDeck(ctx, userID, deck.Deck{
		Name:        req.Name,
		Format:      req.Format,
		CommanderID: commanderID,
	})
	if err != nil {
		return summary, fmt.Errorf("creating deck %q: %w", req.Name, err)
	}
	summary.DecksCreated++

	for _, p := range placements {
		for _, cardID := range p.ownedIDs {
			if err := s.decks.PutCardInDeck(ctx, userID, createdDeck.ID, cardID); err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: could not put %q in the deck: %v", p.line.LineNo, p.line.CardName, err))
				continue
			}
			summary.CardsLinked++
		}
		if p.missing == 0 {
			continue
		}

		colors, cardType, identity := p.resolved.details()
		pending, err := s.decks.AddPendingCard(ctx, userID, createdDeck.ID, deck.PendingCard{
			Name:            p.line.CardName,
			ScryfallID:      p.resolved.ScryfallID,
			SetCode:         p.line.SetCode,
			CollectorNumber: p.line.CollectorNumber,
			Foil:            p.line.Foil,
			Quantity:        p.missing,
			ManaValue:       p.resolved.ManaValue,
			Colors:          colors,
			CardType:        cardType,
			ColorIdentity:   identity,
		})
		if err != nil {
			summary.CardsSkipped += p.missing
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: could not add %q to the deck: %v", p.line.LineNo, p.line.CardName, err))
			continue
		}
		summary.CardsPending += p.missing

		if p.commander && len(p.ownedIDs) == 0 {
			if err := s.decks.SetPendingCommander(ctx, userID, createdDeck.ID, pending.ID); err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not make %q the commander: %v", p.line.CardName, err))
			}
		}
	}

	return summary, nil
}
