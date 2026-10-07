package bulk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"Melrakkiie/Tamiyo/internal/deck"
)

func (s *Service) ExportMoxfieldDeck(ctx context.Context, userID string, deckID string, w io.Writer) error {
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return ErrDeckNotFound
		}
		return err
	}

	cards, err := s.decks.GetDeckCards(ctx, userID, deckID, "updated", true)
	if err != nil {
		return fmt.Errorf("loading deck cards: %w", err)
	}

	groups := make(map[moxfieldGroupKey]int)
	for _, c := range cards {
		groups[moxfieldGroupKey{
			Name:            c.Name,
			SetCode:         c.SetCode,
			CollectorNumber: c.CollectorNumber,
			Foil:            c.Foil,
		}]++
	}

	var commanderKey *moxfieldGroupKey
	if d.CommanderID != nil {
		for _, c := range cards {
			if c.ID == *d.CommanderID {
				key := moxfieldGroupKey{
					Name: c.Name, SetCode: c.SetCode, CollectorNumber: c.CollectorNumber, Foil: c.Foil,
				}
				commanderKey = &key
				break
			}
		}
	}

	keys := make([]moxfieldGroupKey, 0, len(groups))
	for k := range groups {
		if commanderKey != nil && k == *commanderKey {
			continue
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Name != keys[j].Name {
			return keys[i].Name < keys[j].Name
		}
		if keys[i].SetCode != keys[j].SetCode {
			return keys[i].SetCode < keys[j].SetCode
		}
		if keys[i].CollectorNumber != keys[j].CollectorNumber {
			return keys[i].CollectorNumber < keys[j].CollectorNumber
		}
		return !keys[i].Foil && keys[j].Foil
	})

	writeLine := func(k moxfieldGroupKey, quantity int) error {
		foil := ""
		if k.Foil {
			foil = " *F*"
		}
		_, err := fmt.Fprintf(w, "%d %s (%s) %s%s\n", quantity, k.Name, k.SetCode, k.CollectorNumber, foil)
		return err
	}

	if commanderKey != nil {
		if err := writeLine(*commanderKey, groups[*commanderKey]); err != nil {
			return fmt.Errorf("writing commander line: %w", err)
		}
	}
	for _, k := range keys {
		if err := writeLine(k, groups[k]); err != nil {
			return fmt.Errorf("writing line: %w", err)
		}
	}

	return nil
}
