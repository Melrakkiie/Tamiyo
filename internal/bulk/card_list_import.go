package bulk

import (
	"context"
	"fmt"
	"io"

	"Melrakkiie/Tamiyo/internal/card"
)

func (s *Service) ImportCardList(ctx context.Context, userID string, storageID *int, r io.Reader) (Summary, error) {
	lines, err := parseMoxfieldDeckList(r)
	if err != nil {
		return Summary{}, err
	}

	if storageID != nil {
		if err := s.checkStorage(ctx, userID, *storageID); err != nil {
			return Summary{}, err
		}
	}

	resolved, err := s.resolveDeckLines(ctx, lines)
	if err != nil {
		return Summary{}, err
	}

	var summary Summary
	var plan card.ImportPlan
	for _, line := range lines {
		resolvedCard, ok := resolved[deckLineKey(line)]
		if !ok {
			summary.CardsSkipped += line.Quantity
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("line %d: %s not found on scryfall", line.LineNo, describeDeckLine(line)))
			continue
		}

		name, setCode, collectorNumber := line.printing(resolvedCard)
		colors, cardType, identity := resolvedCard.details()
		plan.Copies = append(plan.Copies, copiesOf(card.Card{
			Name:            name,
			ScryfallID:      resolvedCard.ScryfallID,
			SetCode:         setCode,
			CollectorNumber: collectorNumber,
			Foil:            line.Foil,
			StorageID:       storageID,
			ManaValue:       resolvedCard.ManaValue,
			Colors:          colors,
			CardType:        cardType,
			ColorIdentity:   identity,
		}, line.Quantity)...)
	}

	created, err := s.cards.ImportCollection(ctx, userID, plan)
	if err != nil {
		return Summary{}, err
	}
	summary.CardsCreated = len(created.Cards)

	return summary, nil
}
