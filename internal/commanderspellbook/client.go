// Package commanderspellbook finds the combos a deck holds through Commander
// Spellbook's find-my-combos endpoint (ADR 0010).
package commanderspellbook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	findMyCombosURL = "https://backend.commanderspellbook.com/find-my-combos"
	userAgent       = "Tamiyo/1.0 (+https://github.com/Melrakkiie/Tamiyo)"
	requestTimeout  = 10 * time.Second
	retryDelay      = 1 * time.Second
)

// Combo is one combo Commander Spellbook lists for a deck: its id, the names
// of the cards it uses and the names of the features it produces.
type Combo struct {
	ID      string
	Cards   []string
	Results []string
}

// Client calls Commander Spellbook. Its zero value is not usable; build one
// with NewClient.
type Client struct {
	httpClient *http.Client
	url        string
	retryDelay time.Duration
}

func NewClient() *Client {
	return newClient(findMyCombosURL, retryDelay)
}

func newClient(url string, retryDelay time.Duration) *Client {
	return &Client{httpClient: &http.Client{Timeout: requestTimeout}, url: url, retryDelay: retryDelay}
}

// FindCombos returns the combos fully included in a deck whose commander zone
// holds commanders and whose main board holds main, both given as card names.
func (c *Client) FindCombos(ctx context.Context, commanders []string, main []string) ([]Combo, error) {
	return c.fetch(ctx, commanders, main)
}

type wireCard struct {
	Card string `json:"card"`
}

type findRequest struct {
	Commanders []wireCard `json:"commanders"`
	Main       []wireCard `json:"main"`
}

func toWire(names []string) []wireCard {
	cards := make([]wireCard, 0, len(names))
	for _, name := range names {
		cards = append(cards, wireCard{Card: name})
	}
	return cards
}

func (c *Client) fetch(ctx context.Context, commanders []string, main []string) ([]Combo, error) {
	payload, err := json.Marshal(findRequest{Commanders: toWire(commanders), Main: toWire(main)})
	if err != nil {
		return nil, fmt.Errorf("encoding find-my-combos request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("building find-my-combos request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling commander spellbook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Only the "included" list is decoded; the other lists (combos the deck
	// almost has, or would have with other commanders or colors) are dropped.
	var decoded findResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decoding find-my-combos response: %w", err)
	}
	return decoded.combos(), nil
}

type findResponse struct {
	Results struct {
		Included []wireVariant `json:"included"`
	} `json:"results"`
}

type wireVariant struct {
	ID   string `json:"id"`
	Uses []struct {
		Card struct {
			Name string `json:"name"`
		} `json:"card"`
	} `json:"uses"`
	Produces []struct {
		Feature struct {
			Name string `json:"name"`
		} `json:"feature"`
	} `json:"produces"`
}

func (r findResponse) combos() []Combo {
	combos := make([]Combo, 0, len(r.Results.Included))
	for _, v := range r.Results.Included {
		combo := Combo{ID: v.ID, Cards: make([]string, 0, len(v.Uses)), Results: make([]string, 0, len(v.Produces))}
		for _, use := range v.Uses {
			combo.Cards = append(combo.Cards, use.Card.Name)
		}
		for _, produced := range v.Produces {
			combo.Results = append(combo.Results, produced.Feature.Name)
		}
		combos = append(combos, combo)
	}
	return combos
}
