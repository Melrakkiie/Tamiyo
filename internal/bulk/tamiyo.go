package bulk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"Melrakkiie/Tamiyo/internal/deck"
)

const (
	tamiyoFormatVersion = 1

	TamiyoKindCollection = "collection"
	TamiyoKindStorage    = "storage"
	TamiyoKindDeck       = "deck"

	maxTamiyoQuantity = 1000
)

type tamiyoStorage struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type tamiyoCollectionCard struct {
	Name            string  `json:"name"`
	ScryfallID      string  `json:"scryfall_id"`
	SetCode         string  `json:"set_code"`
	CollectorNumber string  `json:"collector_number"`
	Foil            bool    `json:"foil"`
	Proxy           bool    `json:"proxy"`
	Quantity        int     `json:"quantity"`
	Storage         *string `json:"storage"`
}

type tamiyoDeckInfo struct {
	Name   string `json:"name"`
	Format string `json:"format"`
}

type tamiyoDeckCard struct {
	Name            string `json:"name"`
	ScryfallID      string `json:"scryfall_id"`
	SetCode         string `json:"set_code"`
	CollectorNumber string `json:"collector_number"`
	Foil            bool   `json:"foil"`
	Quantity        int    `json:"quantity"`
	Board           string `json:"board"`
	Commander       bool   `json:"commander,omitempty"`
}

type tamiyoCardTags struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type tamiyoFile struct {
	Tamiyo     int              `json:"tamiyo"`
	Kind       string           `json:"kind"`
	ExportedAt string           `json:"exported_at,omitempty"`
	Deck       *tamiyoDeckInfo  `json:"deck,omitempty"`
	Storages   []tamiyoStorage  `json:"storages,omitempty"`
	Cards      json.RawMessage  `json:"cards"`
	Tags       []tamiyoCardTags `json:"tags,omitempty"`
	collection []tamiyoCollectionCard
	deckCards  []tamiyoDeckCard
}

func looksLikeJSON(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))), []byte("{"))
}

func invalidTamiyo(format string, args ...any) error {
	return fmt.Errorf("%w: invalid Tamiyo file: %s", ErrInvalidFile, fmt.Sprintf(format, args...))
}

func parseTamiyoFile(data []byte) (tamiyoFile, error) {
	var file tamiyoFile
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	if err := decoder.Decode(&file); err != nil {
		return file, invalidTamiyo("not JSON: %v", err)
	}
	if file.Tamiyo == 0 {
		return file, invalidTamiyo("the \"tamiyo\" version field is missing")
	}
	if file.Tamiyo > tamiyoFormatVersion {
		return file, invalidTamiyo("made by a newer version of Tamiyo (format %d)", file.Tamiyo)
	}

	switch file.Kind {
	case TamiyoKindCollection, TamiyoKindStorage:
		if err := json.Unmarshal(file.Cards, &file.collection); err != nil {
			return file, invalidTamiyo("cards: %v", err)
		}
		for i, c := range file.collection {
			if err := checkTamiyoCard(i, c.Name, c.ScryfallID, c.Quantity); err != nil {
				return file, err
			}
			if c.Storage != nil && strings.TrimSpace(*c.Storage) == "" {
				file.collection[i].Storage = nil
			}
		}
	case TamiyoKindDeck:
		if err := json.Unmarshal(file.Cards, &file.deckCards); err != nil {
			return file, invalidTamiyo("cards: %v", err)
		}
		for i, c := range file.deckCards {
			if err := checkTamiyoCard(i, c.Name, c.ScryfallID, c.Quantity); err != nil {
				return file, err
			}
			if c.Board == "" {
				file.deckCards[i].Board = deck.BoardMain
			} else if c.Board != deck.BoardMain && c.Board != deck.BoardSideboard && c.Board != deck.BoardConsidering {
				return file, invalidTamiyo("card %d: unknown board %q", i+1, c.Board)
			}
		}
	default:
		return file, invalidTamiyo("unknown kind %q, expected collection, storage or deck", file.Kind)
	}
	return file, nil
}

func checkTamiyoCard(index int, name, scryfallID string, quantity int) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(scryfallID) == "" {
		return invalidTamiyo("card %d: name and scryfall_id are required", index+1)
	}
	if quantity < 1 || quantity > maxTamiyoQuantity {
		return invalidTamiyo("card %d (%s): quantity must be between 1 and %d", index+1, name, maxTamiyoQuantity)
	}
	return nil
}

func newTamiyoFile(kind string) tamiyoFile {
	return tamiyoFile{Tamiyo: tamiyoFormatVersion, Kind: kind, ExportedAt: time.Now().UTC().Format(time.RFC3339)}
}

func writeTamiyoFile(w interface{ Write([]byte) (int, error) }, file tamiyoFile, cards any) error {
	raw, err := json.Marshal(cards)
	if err != nil {
		return fmt.Errorf("encoding cards: %w", err)
	}
	file.Cards = raw
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(file); err != nil {
		return fmt.Errorf("writing the Tamiyo file: %w", err)
	}
	return nil
}
