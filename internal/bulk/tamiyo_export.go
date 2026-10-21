package bulk

import (
	"context"
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
	own, err := s.decks.ReadDeck(ctx, userID, deckID, deck.ReachOwn)
	if err != nil {
		return err
	}
	return s.writeReadDeck(ctx, own, DeckExportOptions{Format: DeckExportTamiyo, WithTags: withTags}, w)
}

func writeTamiyoDeck(w io.Writer, read deck.ReadDeck, withTags bool) error {
	out := make([]tamiyoDeckCard, 0, len(read.Entries))
	for _, e := range read.Entries {
		out = append(out, tamiyoDeckCard{
			Name: e.Name, ScryfallID: e.ScryfallID, SetCode: e.SetCode, CollectorNumber: e.CollectorNumber,
			Foil: e.Foil, Proxy: e.Proxy, Quantity: e.Quantity(), Board: e.Board, Commander: e.IsCommander(),
		})
	}

	file := newTamiyoFile(TamiyoKindDeck)
	file.Deck = &tamiyoDeckInfo{Name: read.Deck.Name, Format: read.Deck.Format}
	if withTags {
		file.Tags = tamiyoTags(read.Entries, read.Tags)
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
