// Package commanderspellbook finds the combos a deck holds through Commander
// Spellbook's find-my-combos endpoint (ADR 0010).
package commanderspellbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	findMyCombosURL = "https://backend.commanderspellbook.com/find-my-combos"
	userAgent       = "Tamiyo/1.0 (+https://github.com/Melrakkiie/Tamiyo)"
	requestTimeout  = 10 * time.Second
	retryDelay      = 1 * time.Second
	maxRetries      = 1
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
	cache      *comboCache
}

func NewClient() *Client {
	return newClient(findMyCombosURL, retryDelay)
}

func newClient(url string, retryDelay time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		url:        url,
		retryDelay: retryDelay,
		cache:      newComboCache(time.Now, cacheMaxEntries),
	}
}

// FindCombos returns the combos fully included in a deck whose commander zone
// holds commanders and whose main board holds main, both given as card names.
// An answer is kept for 24 hours; failures are never kept.
func (c *Client) FindCombos(ctx context.Context, commanders []string, main []string) ([]Combo, error) {
	key := cacheKey(commanders, main)
	if combos, ok := c.cache.get(key); ok {
		return combos, nil
	}
	combos, err := c.fetch(ctx, commanders, main)
	if err != nil {
		return nil, err
	}
	c.cache.put(key, combos)
	return combos, nil
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

	// A 429 is retried once, after a short wait; anything else fails at once.
	for attempt := 0; ; attempt++ {
		combos, err := c.post(ctx, payload)
		if !errors.Is(err, errRateLimited) || attempt == maxRetries {
			return combos, err
		}
		select {
		case <-time.After(c.retryDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

var errRateLimited = errors.New("commander spellbook rate limit reached")

func (c *Client) post(ctx context.Context, payload []byte) ([]Combo, error) {
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
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("commander spellbook returned status %d", resp.StatusCode)
	}

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
