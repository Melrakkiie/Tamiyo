package bulk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"Melrakkiie/Tamiyo/internal/deck"
)

type tamiyoCollectionKey struct {
	name            string
	scryfallID      string
	setCode         string
	collectorNumber string
	foil            bool
	proxy           bool
	storageID       int
}

func (s *Service) ExportTamiyoCollection(ctx context.Context, userID string, storageID *int, w io.Writer) error {
	cards, err := s.exportedCards(ctx, userID, storageID)
	if err != nil {
		return err
	}
	storagesByID, err := s.loadAllStoragesByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading storages: %w", err)
	}

	quantities := make(map[tamiyoCollectionKey]int)
	var order []tamiyoCollectionKey
	usedStorages := make(map[int]bool)
	for _, c := range cards {
		key := tamiyoCollectionKey{
			name: c.Name, scryfallID: c.ScryfallID, setCode: c.SetCode, collectorNumber: c.CollectorNumber,
			foil: c.Foil, proxy: c.Proxy,
		}
		if c.StorageID != nil {
			if _, known := storagesByID[*c.StorageID]; known {
				key.storageID = *c.StorageID
				usedStorages[*c.StorageID] = true
			}
		}
		if _, seen := quantities[key]; !seen {
			order = append(order, key)
		}
		quantities[key]++
	}

	storageName := func(id int) string { return storagesByID[id].Name }
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.storageID != b.storageID {
			if a.storageID == 0 || b.storageID == 0 {
				return b.storageID == 0
			}
			return strings.ToLower(storageName(a.storageID)) < strings.ToLower(storageName(b.storageID))
		}
		if a.name != b.name {
			return a.name < b.name
		}
		if a.setCode != b.setCode {
			return a.setCode < b.setCode
		}
		if a.collectorNumber != b.collectorNumber {
			return a.collectorNumber < b.collectorNumber
		}
		return !a.foil && b.foil
	})

	kind := TamiyoKindCollection
	if storageID != nil {
		kind = TamiyoKindStorage
		usedStorages[*storageID] = true
	}
	file := newTamiyoFile(kind)
	file.Storages = []tamiyoStorage{}
	for id := range usedStorages {
		st := storagesByID[id]
		file.Storages = append(file.Storages, tamiyoStorage{Name: st.Name, Type: st.Type})
	}
	sort.Slice(file.Storages, func(i, j int) bool {
		return strings.ToLower(file.Storages[i].Name) < strings.ToLower(file.Storages[j].Name)
	})

	out := make([]tamiyoCollectionCard, 0, len(order))
	for _, key := range order {
		entry := tamiyoCollectionCard{
			Name: key.name, ScryfallID: key.scryfallID, SetCode: key.setCode, CollectorNumber: key.collectorNumber,
			Foil: key.foil, Proxy: key.proxy, Quantity: quantities[key],
		}
		if key.storageID != 0 {
			name := storageName(key.storageID)
			entry.Storage = &name
		}
		out = append(out, entry)
	}
	return writeTamiyoFile(w, file, out)
}

func (s *Service) ExportTamiyoDeck(ctx context.Context, userID string, deckID string, withTags bool, w io.Writer) error {
	content, err := s.decks.GetDeckContent(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return ErrDeckNotFound
		}
		return fmt.Errorf("loading deck content: %w", err)
	}
	var tags map[string][]string
	if withTags {
		deckTags, err := s.decks.GetCardTags(ctx, userID, content.Deck.ID)
		if err != nil {
			return fmt.Errorf("loading tags: %w", err)
		}
		tags = deckTags.ByCardName()
	}
	return writeTamiyoDeck(w, content.Deck, content.Entries, withTags, tags)
}

// writeTamiyoDeck takes tags keyed by deck.CardNameKey.
func writeTamiyoDeck(w io.Writer, d deck.Deck, entries []deck.DeckEntry, withTags bool, tags map[string][]string) error {
	out := make([]tamiyoDeckCard, 0, len(entries))
	for _, e := range entries {
		out = append(out, tamiyoDeckCard{
			Name: e.Name, ScryfallID: e.ScryfallID, SetCode: e.SetCode, CollectorNumber: e.CollectorNumber,
			Foil: e.Foil, Proxy: e.Proxy, Quantity: e.Quantity(), Board: e.Board, Commander: e.IsCommander(),
		})
	}

	file := newTamiyoFile(TamiyoKindDeck)
	file.Deck = &tamiyoDeckInfo{Name: d.Name, Format: d.Format}
	if withTags {
		file.Tags = tamiyoTags(entries, tags)
	}
	return writeTamiyoFile(w, file, out)
}

func tamiyoTags(entries []deck.DeckEntry, tags map[string][]string) []tamiyoCardTags {
	cards := []tamiyoCardTags{}
	for _, g := range deck.GroupByCardName(entries) {
		if cardTags := tags[deck.CardNameKey(g[0].Name)]; len(cardTags) > 0 {
			cards = append(cards, tamiyoCardTags{Name: g[0].Name, Tags: cardTags})
		}
	}
	sort.Slice(cards, func(i, j int) bool {
		return strings.ToLower(cards[i].Name) < strings.ToLower(cards[j].Name)
	})
	return cards
}
