package scryfall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var collectionURL = "https://api.scryfall.com/cards/collection"
var retryBaseDelay = 1 * time.Second

const (
	batchSize  = 75
	batchDelay = 150 * time.Millisecond
	maxRetries = 5
)

type Identifier struct {
	ID              string
	Set             string
	CollectorNumber string
}

func (id Identifier) toWire() identifier {
	if id.ID != "" {
		return identifier{ID: id.ID}
	}
	return identifier{Set: strings.ToLower(id.Set), CollectorNumber: id.CollectorNumber}
}

type Card struct {
	ID              string
	Name            string
	Set             string
	CollectorNumber string
	ManaCost        string
	CMC             float64
	TypeLine        string
	Colors          []string
	ColorIdentity   []string
	Legalities      map[string]string
}

type identifier struct {
	ID              string `json:"id,omitempty"`
	Set             string `json:"set,omitempty"`
	CollectorNumber string `json:"collector_number,omitempty"`
}

type collectionRequest struct {
	Identifiers []identifier `json:"identifiers"`
}

type wireCard struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Set             string            `json:"set"`
	CollectorNumber string            `json:"collector_number"`
	ManaCost        string            `json:"mana_cost"`
	CMC             float64           `json:"cmc"`
	TypeLine        string            `json:"type_line"`
	Colors          []string          `json:"colors"`
	ColorIdentity   []string          `json:"color_identity"`
	Legalities      map[string]string `json:"legalities"`
}

type collectionResponse struct {
	Data     []wireCard   `json:"data"`
	NotFound []identifier `json:"not_found"`
	Warnings []string     `json:"warnings,omitempty"`
	Details  string       `json:"details,omitempty"`
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) Fetch(ctx context.Context, identifiers []Identifier) ([]Card, error) {
	var found []Card

	for start := 0; start < len(identifiers); start += batchSize {
		end := start + batchSize
		if end > len(identifiers) {
			end = len(identifiers)
		}

		if start > 0 {
			time.Sleep(batchDelay)
		}

		batch, err := c.fetchBatch(ctx, identifiers[start:end])
		if err != nil {
			return nil, err
		}
		found = append(found, batch...)
	}

	return found, nil
}

func (c *Client) fetchBatch(ctx context.Context, batch []Identifier) ([]Card, error) {
	reqBody := collectionRequest{Identifiers: make([]identifier, len(batch))}
	for i, id := range batch {
		reqBody.Identifiers[i] = id.toWire()
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("encoding scryfall request: %w", err)
	}

	var lastErr error
	retryAfter := retryBaseDelay

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(retryAfter):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := c.doRequest(ctx, body)
		if err != nil {
			return nil, err // network-level failure: not worth retrying the same way as a 429
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
				retryAfter = ra
			} else {
				retryAfter *= 2
			}
			lastErr = fmt.Errorf("scryfall returned status 429 (rate limited)")
			_ = resp.Body.Close()
			continue
		}

		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("scryfall returned status %d", resp.StatusCode)
		}

		var parsed collectionResponse
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return nil, fmt.Errorf("decoding scryfall response: %w", err)
		}

		cards := make([]Card, len(parsed.Data))
		for i, wc := range parsed.Data {
			cards[i] = Card(wc)
		}
		return cards, nil
	}

	return nil, fmt.Errorf("gave up after %d retries: %w", maxRetries, lastErr)
}

func (c *Client) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, collectionURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building scryfall request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Tamiyo/1.0 (+https://github.com/Melrakkiie/Tamiyo)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling scryfall: %w", err)
	}
	return resp, nil
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	seconds, err := strconv.Atoi(header)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
