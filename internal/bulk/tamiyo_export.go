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

type tamiyoDeckKey struct {
	name            string
	scryfallID      string
	setCode         string
	collectorNumber string
	foil            bool
	board           string
	commander       bool
}

func (s *Service) ExportTamiyoDeck(ctx context.Context, userID string, deckID string, withTags bool, w io.Writer) error {
	d, err := s.decks.GetDeck(ctx, userID, deckID)
	if err != nil {
		if errors.Is(err, deck.ErrNotFound) {
			return ErrDeckNotFound
		}
		return err
	}
	cards, err := s.decks.GetDeckCards(ctx, userID, d.ID, "name", false)
	if err != nil {
		return fmt.Errorf("loading deck cards: %w", err)
	}
	pending, err := s.decks.GetPendingCards(ctx, userID, d.ID)
	if err != nil {
		return fmt.Errorf("loading pending cards: %w", err)
	}

	quantities := make(map[tamiyoDeckKey]int)
	var order []tamiyoDeckKey
	add := func(key tamiyoDeckKey, quantity int) {
		if key.board == "" {
			key.board = deck.BoardMain
		}
		if _, seen := quantities[key]; !seen {
			order = append(order, key)
		}
		quantities[key] += quantity
	}
	for _, c := range cards {
		add(tamiyoDeckKey{
			name: c.Name, scryfallID: c.ScryfallID, setCode: c.SetCode, collectorNumber: c.CollectorNumber,
			foil: c.Foil, board: c.Board, commander: d.CommanderID != nil && *d.CommanderID == c.ID,
		}, 1)
	}
	for _, p := range pending {
		key := tamiyoDeckKey{
			name: p.Name, scryfallID: p.ScryfallID, setCode: p.SetCode, collectorNumber: p.CollectorNumber,
			foil: p.Foil, board: p.Board,
		}
		quantity := p.Quantity
		if d.CommanderPendingID != nil && *d.CommanderPendingID == p.ID {
			commander := key
			commander.commander = true
			add(commander, 1)
			quantity--
		}
		if quantity > 0 {
			add(key, quantity)
		}
	}

	boardOrder := map[string]int{deck.BoardMain: 0, deck.BoardSideboard: 1, deck.BoardConsidering: 2}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.commander != b.commander {
			return a.commander
		}
		if a.board != b.board {
			return boardOrder[a.board] < boardOrder[b.board]
		}
		if a.name != b.name {
			return a.name < b.name
		}
		if a.setCode != b.setCode {
			return a.setCode < b.setCode
		}
		return !a.foil && b.foil
	})

	out := make([]tamiyoDeckCard, 0, len(order))
	for _, key := range order {
		out = append(out, tamiyoDeckCard{
			Name: key.name, ScryfallID: key.scryfallID, SetCode: key.setCode, CollectorNumber: key.collectorNumber,
			Foil: key.foil, Quantity: quantities[key], Board: key.board, Commander: key.commander,
		})
	}

	file := newTamiyoFile(TamiyoKindDeck)
	file.Deck = &tamiyoDeckInfo{Name: d.Name, Format: d.Format}
	if withTags {
		tags, err := s.decks.GetCardTags(ctx, userID, d.ID)
		if err != nil {
			return fmt.Errorf("loading tags: %w", err)
		}
		file.Tags = []tamiyoCardTags{}
		for _, c := range tags.Cards {
			if len(c.Tags) > 0 {
				file.Tags = append(file.Tags, tamiyoCardTags{Name: c.Name, Tags: c.Tags})
			}
		}
	}
	return writeTamiyoFile(w, file, out)
}
