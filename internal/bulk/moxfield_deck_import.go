package bulk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
)

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
	return resolved.Name, line.SetCode, line.CollectorNumber
}

// placementLines turns the resolved lines into placement lines. The first
// two commander lines of the main board ask for the commander slots; lines
// Scryfall doesn't know are skipped with a warning.
func placementLines(lines []moxfieldDeckLine, resolved map[string]ResolvedCard) ([]deck.PlacementLine, Summary) {
	var summary Summary
	placed := make([]deck.PlacementLine, 0, len(lines))
	commanders := maxCommanders
	for _, line := range lines {
		commander := commanders > 0 && line.Commander && deck.InMainBoard(line.Board)
		if commander {
			commanders--
		}
		rc, ok := resolved[deckLineKey(line)]
		if !ok {
			summary.CardsSkipped += line.Quantity
			label := ""
			if commander {
				label = "commander "
			}
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: %s%s not found on scryfall", line.LineNo, label, describeDeckLine(line)))
			continue
		}
		colors, cardType, identity := rc.details()
		name, setCode, collectorNumber := line.printing(rc)
		placed = append(placed, deck.PlacementLine{
			Printing: deck.Printing{
				Name: name, ScryfallID: rc.ScryfallID, SetCode: setCode, CollectorNumber: collectorNumber,
				ManaValue: rc.ManaValue, Colors: colors, CardType: cardType, ColorIdentity: identity,
			},
			AnyPrinting: line.SetCode == "",
			Foil:        line.Foil,
			Proxy:       line.Proxy,
			Board:       line.Board,
			Quantity:    line.Quantity,
			Commander:   commander,
		})
	}
	return placed, summary
}

func (s *Service) placeLines(ctx context.Context, userID string, deckID string, plan deck.PlacementPlan, summary *Summary) error {
	placed, err := s.decks.PlaceCards(ctx, userID, deckID, plan)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return ErrDeckNotFound
		}
		return err
	}
	summary.CardsLinked += placed.CardsLinked
	summary.CardsPending += placed.CardsPending
	for _, skipped := range placed.SkippedTags {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not tag %q: %v", skipped.CardName, skipped.Err))
	}
	return nil
}

func lineTags(lines []moxfieldDeckLine, resolved map[string]ResolvedCard) []deck.TaggedCard {
	var tags []deck.TaggedCard
	index := make(map[string]int)
	for _, line := range lines {
		if len(line.Tags) == 0 {
			continue
		}
		name := line.CardName
		if rc, ok := resolved[deckLineKey(line)]; ok {
			name = rc.Name
		}
		key := deck.CardNameKey(name)
		i, seen := index[key]
		if !seen {
			i = len(tags)
			index[key] = i
			tags = append(tags, deck.TaggedCard{Name: name})
		}
		for _, tag := range line.Tags {
			if !containsFold(tags[i].Tags, tag) {
				tags[i].Tags = append(tags[i].Tags, tag)
			}
		}
	}
	return tags
}

func (s *Service) ImportIntoDeck(ctx context.Context, userID string, deckID string, opts DeckImportOptions, r io.Reader) (Summary, error) {
	if _, err := s.decks.GetDeck(ctx, userID, deckID); err != nil {
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
	var tags []deck.TaggedCard
	tamiyoFile := looksLikeJSON(data)
	if tamiyoFile {
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
		lines = tamiyoDeckLines(file)
		tags = file.taggedCards()
	} else {
		if lines, err = parseMoxfieldDeckList(bytes.NewReader(data)); err != nil {
			return Summary{}, err
		}
		if opts.CommanderFromFirstLine {
			markFirstLineCommander(lines)
		}
	}

	resolved, err := s.resolveDeckLines(ctx, lines)
	if err != nil {
		return Summary{}, err
	}
	if !tamiyoFile {
		tags = lineTags(lines, resolved)
	}

	placed, summary := placementLines(lines, resolved)
	plan := deck.PlacementPlan{Lines: placed, Tags: tags, IncludeLockedStorages: opts.IncludeLockedStorages}
	if err := s.placeLines(ctx, userID, deckID, plan, &summary); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func markFirstLineCommander(lines []moxfieldDeckLine) {
	for _, line := range lines {
		if line.Commander {
			return
		}
	}
	for i := range lines {
		if deck.InMainBoard(lines[i].Board) {
			lines[i].Commander = true
			return
		}
	}
}
