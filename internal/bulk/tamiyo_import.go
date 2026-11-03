package bulk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/deck"
)

var (
	ErrTamiyoDeckFile       = errors.New("this Tamiyo file is a deck: import it from a deck page")
	ErrTamiyoCollectionFile = errors.New("this Tamiyo file is a collection or a storage: import it from the collection or a storage page")
)

const defaultTamiyoStorageType = "binder"

func readImport(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	return data, nil
}

func (s *Service) ImportTamiyoCollection(ctx context.Context, userID string, targetStorageID *int, r io.Reader) (Summary, error) {
	data, err := readImport(r)
	if err != nil {
		return Summary{}, err
	}
	file, err := parseTamiyoFile(data)
	if err != nil {
		return Summary{}, err
	}
	if file.Kind == TamiyoKindDeck {
		return Summary{}, ErrTamiyoDeckFile
	}

	var storageCache map[string]int
	if targetStorageID != nil {
		if err := s.checkStorage(ctx, userID, *targetStorageID); err != nil {
			return Summary{}, err
		}
	} else if storageCache, err = s.loadStorageCache(ctx, userID); err != nil {
		return Summary{}, fmt.Errorf("loading existing storages: %w", err)
	}
	storageTypes := make(map[string]string, len(file.Storages))
	for _, st := range file.Storages {
		storageTypes[st.Name] = st.Type
	}

	identifiers := make([]CardIdentifier, 0, len(file.collection))
	for _, c := range file.collection {
		identifiers = append(identifiers, CardIdentifier{ScryfallID: c.ScryfallID})
	}
	resolved, err := s.scryfall.Resolve(ctx, dedupeIdentifiers(identifiers))
	if err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrScryfallUnavailable, err)
	}

	var summary Summary
	plan := card.ImportPlan{}
	planned := map[string]int{} // storage name -> index in plan.Storages
	for i, c := range file.collection {
		rc, ok := resolved[resolveKeyByID(c.ScryfallID)]
		if !ok {
			summary.CardsSkipped += c.Quantity
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("card %d: %q (%s #%s) not found on scryfall", i+1, c.Name, c.SetCode, c.CollectorNumber))
			continue
		}

		colors, cardType, identity := rc.details()
		base := card.Card{
			Name:            rc.Name,
			ScryfallID:      rc.ScryfallID,
			SetCode:         rc.SetCode,
			CollectorNumber: rc.CollectorNumber,
			Foil:            c.Foil,
			Proxy:           c.Proxy,
			StorageID:       targetStorageID,
			ManaValue:       rc.ManaValue,
			Colors:          colors,
			CardType:        cardType,
			ColorIdentity:   identity,
		}
		copies := copiesOf(base, c.Quantity)
		if targetStorageID == nil && c.Storage != nil {
			name := strings.TrimSpace(*c.Storage)
			if id, ok := storageCache[name]; ok {
				for j := range copies {
					copies[j].Card.StorageID = &id
				}
			} else {
				idx, ok := planned[name]
				if !ok {
					storageType := storageTypes[*c.Storage]
					if storageType == "" {
						storageType = defaultTamiyoStorageType
					}
					idx = len(plan.Storages)
					planned[name] = idx
					plan.Storages = append(plan.Storages, card.NewStorage{Name: name, Type: storageType})
				}
				for j := range copies {
					copies[j].NewStorage = &idx
				}
			}
		}
		plan.Copies = append(plan.Copies, copies...)
	}

	result, err := s.cards.ImportCollection(ctx, userID, plan)
	if err != nil {
		return Summary{}, err
	}
	summary.CardsCreated = len(result.Cards)
	summary.StoragesCreated = len(result.StorageIDs)
	return summary, nil
}

func tamiyoDeckLines(file tamiyoFile) []moxfieldDeckLine {
	lines := make([]moxfieldDeckLine, 0, len(file.deckCards))
	commanders := 0
	for i, c := range file.deckCards {
		line := moxfieldDeckLine{
			LineNo:          i + 1,
			Quantity:        c.Quantity,
			CardName:        c.Name,
			SetCode:         c.SetCode,
			CollectorNumber: c.CollectorNumber,
			Foil:            c.Foil,
			Proxy:           proxyMatch(c.Proxy),
			Board:           c.Board,
		}
		if c.Commander && commanders < maxCommanders && deck.InMainBoard(c.Board) {
			line.Commander = true
			lines = append(lines[:commanders], append([]moxfieldDeckLine{line}, lines[commanders:]...)...)
			commanders++
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func (file tamiyoFile) taggedCards() []deck.TaggedCard {
	var tags []deck.TaggedCard
	for _, t := range file.Tags {
		tags = append(tags, deck.TaggedCard{Name: t.Name, Tags: t.Tags})
	}
	return tags
}

func containsFold(values []string, value string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}
