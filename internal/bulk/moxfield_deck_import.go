package bulk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

type ownedCopies struct {
	byPrinting map[string][]card.Card
	byName     map[string][]card.Card
	inDeck     map[int]bool
	used       map[int]bool
}

func (o *ownedCopies) takePrinting(scryfallID string, foil bool, quantity int) []int {
	return o.take(o.byPrinting[strings.ToLower(scryfallID)], foil, quantity)
}

func (o *ownedCopies) takeName(name string, foil bool, quantity int) []int {
	return o.take(o.byName[deck.CardNameKey(name)], foil, quantity)
}

func (o *ownedCopies) take(copies []card.Card, foil bool, quantity int) []int {
	candidates := append([]card.Card(nil), copies...)
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

func (s *Service) loadOwnedCopies(ctx context.Context, userID string, targetDeckID string) (*ownedCopies, error) {
	cards, err := s.loadAllCards(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loading the collection: %w", err)
	}
	owned := &ownedCopies{
		byPrinting: make(map[string][]card.Card),
		byName:     make(map[string][]card.Card),
		inDeck:     make(map[int]bool),
		used:       make(map[int]bool),
	}
	for _, c := range cards {
		key := strings.ToLower(c.ScryfallID)
		owned.byPrinting[key] = append(owned.byPrinting[key], c)
		full, front := deck.CardNameKey(c.Name), deck.CardNameKey(frontFaceName(c.Name))
		owned.byName[full] = append(owned.byName[full], c)
		if front != full {
			owned.byName[front] = append(owned.byName[front], c)
		}
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
				if d.ID == targetDeckID {
					owned.used[c.ID] = true
				}
			}
		}
		if len(decks) == 0 || page*limit >= total {
			return owned, nil
		}
	}
}

func scryfallName(name string) string {
	if strings.Contains(name, "//") {
		return name
	}
	return strings.ReplaceAll(name, " / ", " // ")
}

func deckLineKey(line moxfieldDeckLine) string {
	if line.SetCode == "" {
		return resolveKeyByName(line.CardName)
	}
	return resolveKey(line.SetCode, line.CollectorNumber)
}

func describeDeckLine(line moxfieldDeckLine) string {
	if line.SetCode == "" {
		return fmt.Sprintf("%q", line.CardName)
	}
	return fmt.Sprintf("%q (%s #%s)", line.CardName, line.SetCode, line.CollectorNumber)
}

func (s *Service) resolveDeckLines(ctx context.Context, lines []moxfieldDeckLine) (map[string]ResolvedCard, error) {
	identifiers := make([]CardIdentifier, len(lines))
	for i, line := range lines {
		if line.SetCode == "" {
			identifiers[i] = CardIdentifier{Name: scryfallName(line.CardName)}
		} else {
			identifiers[i] = CardIdentifier{SetCode: line.SetCode, CollectorNumber: line.CollectorNumber}
		}
	}
	resolved, err := s.scryfall.Resolve(ctx, dedupeIdentifiers(identifiers))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
	}
	return resolved, nil
}

func (line moxfieldDeckLine) printing(resolved ResolvedCard) (name, setCode, collectorNumber string) {
	if line.SetCode == "" {
		return resolved.Name, resolved.SetCode, resolved.CollectorNumber
	}
	return line.CardName, line.SetCode, line.CollectorNumber
}

type deckLinePlacement struct {
	line      moxfieldDeckLine
	resolved  ResolvedCard
	ownedIDs  []int
	missing   int
	commander bool
}

func (s *Service) placeDeckLines(ctx context.Context, userID string, lines []moxfieldDeckLine, commanderFromFirstLine bool, targetDeckID string) ([]deckLinePlacement, Summary, error) {
	resolved, err := s.resolveDeckLines(ctx, lines)
	if err != nil {
		return nil, Summary{}, err
	}
	return s.placeResolvedLines(ctx, userID, lines, resolved, commanderFromFirstLine, targetDeckID)
}

func (s *Service) placeResolvedLines(ctx context.Context, userID string, lines []moxfieldDeckLine, resolved map[string]ResolvedCard, commanderFromFirstLine bool, targetDeckID string) ([]deckLinePlacement, Summary, error) {
	owned, err := s.loadOwnedCopies(ctx, userID, targetDeckID)
	if err != nil {
		return nil, Summary{}, err
	}

	var summary Summary
	placements := make([]deckLinePlacement, 0, len(lines))
	commanderPending := commanderFromFirstLine
	for _, line := range lines {
		commander := commanderPending && deck.InMainBoard(line.Board)
		if commander {
			commanderPending = false
		}
		resolvedCard, ok := resolved[deckLineKey(line)]
		if !ok {
			summary.CardsSkipped += line.Quantity
			label := ""
			if commander {
				label = "commander "
			}
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: %s%s not found on scryfall", line.LineNo, label, describeDeckLine(line)))
			continue
		}
		var ownedIDs []int
		if line.SetCode == "" {
			ownedIDs = owned.takeName(line.CardName, line.Foil, line.Quantity)
		} else {
			ownedIDs = owned.takePrinting(resolvedCard.ScryfallID, line.Foil, line.Quantity)
		}
		placements = append(placements, deckLinePlacement{
			line:      line,
			resolved:  resolvedCard,
			ownedIDs:  ownedIDs,
			missing:   line.Quantity - len(ownedIDs),
			commander: commander,
		})
	}
	return placements, summary, nil
}

func (s *Service) fillDeck(ctx context.Context, userID string, deckID string, placements []deckLinePlacement, summary *Summary) {
	for _, p := range placements {
		linked := 0
		for _, cardID := range p.ownedIDs {
			if err := s.decks.PutCardInDeck(ctx, userID, deckID, cardID, p.line.Board); err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: could not put %q in the deck: %v", p.line.LineNo, p.line.CardName, err))
				continue
			}
			if linked == 0 && p.commander {
				if err := s.decks.SetCardCommander(ctx, userID, deckID, cardID); err != nil {
					summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not make %q the commander: %v", p.line.CardName, err))
				}
			}
			linked++
			summary.CardsLinked++
		}
		if p.missing == 0 {
			continue
		}

		colors, cardType, identity := p.resolved.details()
		name, setCode, collectorNumber := p.line.printing(p.resolved)
		pending, err := s.decks.AddPendingCard(ctx, userID, deckID, deck.PendingCard{
			Name:            name,
			ScryfallID:      p.resolved.ScryfallID,
			SetCode:         setCode,
			CollectorNumber: collectorNumber,
			Foil:            p.line.Foil,
			Quantity:        p.missing,
			ManaValue:       p.resolved.ManaValue,
			Colors:          colors,
			CardType:        cardType,
			ColorIdentity:   identity,
			Board:           p.line.Board,
		})
		if err != nil {
			summary.CardsSkipped += p.missing
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: could not add %q to the deck: %v", p.line.LineNo, p.line.CardName, err))
			continue
		}
		summary.CardsPending += p.missing

		if p.commander && linked == 0 {
			if err := s.decks.SetPendingCommander(ctx, userID, deckID, pending.ID); err != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not make %q the commander: %v", p.line.CardName, err))
			}
		}
	}
}

func lineTags(lines []moxfieldDeckLine) []tamiyoCardTags {
	var tags []tamiyoCardTags
	index := make(map[string]int)
	for _, line := range lines {
		if len(line.Tags) == 0 {
			continue
		}
		key := deck.CardNameKey(line.CardName)
		i, seen := index[key]
		if !seen {
			i = len(tags)
			index[key] = i
			tags = append(tags, tamiyoCardTags{Name: line.CardName})
		}
		for _, tag := range line.Tags {
			if !containsFold(tags[i].Tags, tag) {
				tags[i].Tags = append(tags[i].Tags, tag)
			}
		}
	}
	return tags
}

func (s *Service) ImportIntoDeck(ctx context.Context, userID string, deckID string, commanderFromFirstLine bool, r io.Reader) (Summary, error) {
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return Summary{}, ErrDeckNotFound
		}
		return Summary{}, err
	}

	data, err := readImport(r)
	if err != nil {
		return Summary{}, err
	}

	var lines []moxfieldDeckLine
	var tags []tamiyoCardTags
	if looksLikeJSON(data) {
		file, err := parseTamiyoFile(data)
		if err != nil {
			return Summary{}, err
		}
		if file.Kind != TamiyoKindDeck {
			return Summary{}, ErrTamiyoCollectionFile
		}
		if len(file.deckCards) == 0 {
			return Summary{}, invalidTamiyo("the deck has no cards")
		}
		lines, commanderFromFirstLine = tamiyoDeckLines(file)
		tags = file.Tags
	} else {
		if lines, err = parseMoxfieldDeckList(bytes.NewReader(data)); err != nil {
			return Summary{}, err
		}
		tags = lineTags(lines)
	}

	hasCommander := d.CommanderID != nil || d.CommanderPendingID != nil
	placements, summary, err := s.placeDeckLines(ctx, userID, lines, commanderFromFirstLine && !hasCommander, d.ID)
	if err != nil {
		return Summary{}, err
	}

	s.fillDeck(ctx, userID, d.ID, placements, &summary)
	if err := s.applyTamiyoTags(ctx, userID, d.ID, tags, &summary); err != nil {
		return summary, err
	}
	return summary, nil
}
