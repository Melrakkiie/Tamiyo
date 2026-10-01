package bulk

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

var scryfallCollectionURL = "https://api.scryfall.com/cards/collection"
var scryfallRetryBaseDelay = 1 * time.Second

const (
	scryfallBatchSize = 75

	scryfallBatchDelay = 150 * time.Millisecond

	scryfallMaxRetries = 5
)

type ScryfallClient struct {
	httpClient *http.Client
}

func NewScryfallClient() *ScryfallClient {
	return &ScryfallClient{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

type scryfallIdentifier struct {
	Set             string `json:"set"`
	CollectorNumber string `json:"collector_number"`
}

type scryfallCollectionRequest struct {
	Identifiers []scryfallIdentifier `json:"identifiers"`
}

type scryfallCard struct {
	ID              string `json:"id"`
	Set             string `json:"set"`
	CollectorNumber string `json:"collector_number"`
}

type scryfallCollectionResponse struct {
	Data     []scryfallCard       `json:"data"`
	NotFound []scryfallIdentifier `json:"not_found"`
	Warnings []string             `json:"warnings,omitempty"`
	Details  string               `json:"details,omitempty"`
}

func (c *ScryfallClient) Resolve(ctx context.Context, identifiers []CardIdentifier) (map[string]string, error) {
	resolved := make(map[string]string, len(identifiers))

	for start := 0; start < len(identifiers); start += scryfallBatchSize {
		end := start + scryfallBatchSize
		if end > len(identifiers) {
			end = len(identifiers)
		}

		if start > 0 {
			time.Sleep(scryfallBatchDelay)
		}

		if err := c.resolveBatch(ctx, identifiers[start:end], resolved); err != nil {
			return nil, err
		}
	}

	return resolved, nil
}

func (c *ScryfallClient) resolveBatch(ctx context.Context, batch []CardIdentifier, resolved map[string]string) error {
	reqBody := scryfallCollectionRequest{Identifiers: make([]scryfallIdentifier, len(batch))}
	for i, id := range batch {
		reqBody.Identifiers[i] = scryfallIdentifier{
			Set:             strings.ToLower(id.SetCode),
			CollectorNumber: id.CollectorNumber,
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("encoding scryfall request: %w", err)
	}

	var lastErr error
	retryAfter := scryfallRetryBaseDelay

	for attempt := 0; attempt <= scryfallMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(retryAfter):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		resp, err := c.doRequest(ctx, body)
		if err != nil {
			return err // network-level failure: not worth retrying the same way as a 429
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
			return fmt.Errorf("scryfall returned status %d", resp.StatusCode)
		}

		var parsed scryfallCollectionResponse
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return fmt.Errorf("decoding scryfall response: %w", err)
		}

		for _, found := range parsed.Data {
			resolved[resolveKey(found.Set, found.CollectorNumber)] = found.ID
		}

		return nil
	}

	return fmt.Errorf("gave up after %d retries: %w", scryfallMaxRetries, lastErr)
}

func (c *ScryfallClient) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scryfallCollectionURL, bytes.NewReader(body))
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
