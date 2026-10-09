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
	for i, c := range file.collection {
		rc, ok := resolved[resolveKeyByID(c.ScryfallID)]
		if !ok {
			summary.CardsSkipped += c.Quantity
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("card %d: %q (%s #%s) not found on scryfall", i+1, c.Name, c.SetCode, c.CollectorNumber))
			continue
		}

		storageID := targetStorageID
		if targetStorageID == nil && c.Storage != nil {
			name := strings.TrimSpace(*c.Storage)
			storageType := storageTypes[*c.Storage]
			if storageType == "" {
				storageType = defaultTamiyoStorageType
			}
			id, created, err := s.getOrCreateStorage(ctx, userID, storageCache, name, storageType)
			if err != nil {
				return summary, fmt.Errorf("card %d: creating storage %q: %w", i+1, name, err)
			}
			if created {
				summary.StoragesCreated++
			}
			storageID = &id
		}

		colors, cardType, identity := rc.details()
		for copy := 0; copy < c.Quantity; copy++ {
			if _, err := s.cards.CreateCard(ctx, userID, card.Card{
				Name:            rc.Name,
				ScryfallID:      rc.ScryfallID,
				SetCode:         rc.SetCode,
				CollectorNumber: rc.CollectorNumber,
				Foil:            c.Foil,
				Proxy:           c.Proxy,
				StorageID:       storageID,
				ManaValue:       rc.ManaValue,
				Colors:          colors,
				CardType:        cardType,
				ColorIdentity:   identity,
			}); err != nil {
				summary.CardsSkipped++
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("card %d: could not create %q: %v", i+1, c.Name, err))
				continue
			}
			summary.CardsCreated++
		}
	}
	return summary, nil
}

func tamiyoDeckLines(file tamiyoFile) ([]moxfieldDeckLine, bool) {
	lines := make([]moxfieldDeckLine, 0, len(file.deckCards))
	hasCommander := false
	for i, c := range file.deckCards {
		line := moxfieldDeckLine{
			LineNo:          i + 1,
			Quantity:        c.Quantity,
			CardName:        c.Name,
			SetCode:         c.SetCode,
			CollectorNumber: c.CollectorNumber,
			Foil:            c.Foil,
			Board:           c.Board,
		}
		if c.Commander && !hasCommander && deck.InMainBoard(c.Board) {
			hasCommander = true
			lines = append([]moxfieldDeckLine{line}, lines...)
			continue
		}
		lines = append(lines, line)
	}
	return lines, hasCommander
}

func (s *Service) applyTamiyoTags(ctx context.Context, userID string, deckID string, tags []tamiyoCardTags, summary *Summary) error {
	if len(tags) == 0 {
		return nil
	}
	current, err := s.decks.GetCardTags(ctx, userID, deckID)
	if err != nil {
		return fmt.Errorf("loading tags: %w", err)
	}
	existing := current.ByCardName()
	for _, entry := range tags {
		merged := append([]string{}, existing[deck.CardNameKey(entry.Name)]...)
		for _, tag := range entry.Tags {
			if !containsFold(merged, tag) {
				merged = append(merged, tag)
			}
		}
		if _, err := s.decks.SetCardTags(ctx, userID, deckID, entry.Name, merged); err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("could not tag %q: %v", entry.Name, err))
		}
	}
	return nil
}

func containsFold(values []string, value string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}
