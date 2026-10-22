package scryfall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var collectionURL = "https://api.scryfall.com/cards/collection"
var retryBaseDelay = 1 * time.Second

// errRejected is Scryfall's 400 on a batch, which an identifier it deems
// invalid triggers instead of a not_found entry.
var errRejected = errors.New("scryfall rejected the batch")

const (
	batchSize  = 75
	maxRetries = 5
)

type Identifier struct {
	ID              string
	Set             string
	CollectorNumber string
	Name            string
}

func (id Identifier) toWire() identifier {
	if id.ID != "" {
		return identifier{ID: id.ID}
	}
	if id.Name != "" {
		return identifier{Name: id.Name}
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
	OracleText      string
	Rarity          string
	Keywords        []string
	Colors          []string
	ColorIdentity   []string
	Legalities      map[string]string
	Powers          []string
	Toughnesses     []string
	Prices          Prices
	GameChanger     bool
}

type Prices struct {
	EUR       *float64
	EURFoil   *float64
	USD       *float64
	USDFoil   *float64
	USDEtched *float64
}

type identifier struct {
	ID              string `json:"id,omitempty"`
	Set             string `json:"set,omitempty"`
	CollectorNumber string `json:"collector_number,omitempty"`
	Name            string `json:"name,omitempty"`
}

type collectionRequest struct {
	Identifiers []identifier `json:"identifiers"`
}

type wireCard struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Layout          string            `json:"layout"`
	Set             string            `json:"set"`
	CollectorNumber string            `json:"collector_number"`
	ManaCost        string            `json:"mana_cost"`
	CMC             *float64          `json:"cmc"`
	TypeLine        string            `json:"type_line"`
	OracleText      string            `json:"oracle_text"`
	Rarity          string            `json:"rarity"`
	Keywords        []string          `json:"keywords"`
	Power           string            `json:"power"`
	Toughness       string            `json:"toughness"`
	Colors          []string          `json:"colors"`
	ColorIdentity   []string          `json:"color_identity"`
	Legalities      map[string]string `json:"legalities"`
	CardFaces       []wireCardFace    `json:"card_faces"`
	Prices          wirePrices        `json:"prices"`
	GameChanger     bool              `json:"game_changer"`
}

type wirePrices struct {
	EUR       *string `json:"eur"`
	EURFoil   *string `json:"eur_foil"`
	USD       *string `json:"usd"`
	USDFoil   *string `json:"usd_foil"`
	USDEtched *string `json:"usd_etched"`
}

func (w wirePrices) toPrices() Prices {
	return Prices{EUR: parsePrice(w.EUR), EURFoil: parsePrice(w.EURFoil), USD: parsePrice(w.USD), USDFoil: parsePrice(w.USDFoil), USDEtched: parsePrice(w.USDEtched)}
}

func parsePrice(raw *string) *float64 {
	if raw == nil {
		return nil
	}
	value, err := strconv.ParseFloat(*raw, 64)
	if err != nil {
		return nil
	}
	return &value
}

type wireCardFace struct {
	ManaCost   string   `json:"mana_cost"`
	CMC        *float64 `json:"cmc"`
	TypeLine   string   `json:"type_line"`
	OracleText string   `json:"oracle_text"`
	Colors     []string `json:"colors"`
	Power      string   `json:"power"`
	Toughness  string   `json:"toughness"`
}

func (w wireCard) toCard() Card {
	colors := w.Colors
	if colors == nil {
		for _, face := range w.CardFaces {
			colors = append(colors, face.Colors...)
		}
		colors = dedupe(colors)
	}

	powers := nonEmpty(w.Power)
	toughnesses := nonEmpty(w.Toughness)
	for _, face := range w.CardFaces {
		powers = append(powers, nonEmpty(face.Power)...)
		toughnesses = append(toughnesses, nonEmpty(face.Toughness)...)
	}

	return Card{
		ID:              w.ID,
		Name:            w.name(),
		Set:             w.Set,
		CollectorNumber: w.CollectorNumber,
		ManaCost:        w.manaCost(),
		CMC:             w.cmc(),
		TypeLine:        w.typeLine(),
		OracleText:      w.oracleText(),
		Rarity:          w.Rarity,
		Keywords:        w.Keywords,
		Colors:          colors,
		ColorIdentity:   w.ColorIdentity,
		Legalities:      w.Legalities,
		Powers:          dedupe(powers),
		Toughnesses:     dedupe(toughnesses),
		Prices:          w.Prices.toPrices(),
		GameChanger:     w.GameChanger,
	}
}

func (w wireCard) name() string {
	if w.Layout != "reversible_card" {
		return w.Name
	}
	return strings.Join(dedupe(strings.Split(w.Name, " // ")), " // ")
}

func (w wireCard) manaCost() string {
	if w.ManaCost == "" && len(w.CardFaces) > 0 {
		return w.CardFaces[0].ManaCost
	}
	return w.ManaCost
}

func (w wireCard) cmc() float64 {
	if w.CMC != nil {
		return *w.CMC
	}
	if len(w.CardFaces) > 0 && w.CardFaces[0].CMC != nil {
		return *w.CardFaces[0].CMC
	}
	return 0
}

func (w wireCard) typeLine() string {
	if w.TypeLine != "" {
		return w.TypeLine
	}
	lines := make([]string, 0, len(w.CardFaces))
	for _, face := range w.CardFaces {
		if face.TypeLine != "" {
			lines = append(lines, face.TypeLine)
		}
	}
	return strings.Join(lines, " // ")
}

func (w wireCard) oracleText() string {
	if w.OracleText != "" || len(w.CardFaces) == 0 {
		return w.OracleText
	}
	texts := make([]string, 0, len(w.CardFaces))
	for _, face := range w.CardFaces {
		texts = append(texts, face.OracleText)
	}
	return strings.Join(texts, "\n//\n")
}

func nonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func dedupe(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}
	return unique
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

		batch, err := c.fetchSplitting(ctx, identifiers[start:end])
		if err != nil {
			return nil, err
		}
		found = append(found, batch...)
	}

	return found, nil
}

// fetchSplitting halves a batch Scryfall rejects until it isolates the
// rejected identifiers, which it then treats as not found.
func (c *Client) fetchSplitting(ctx context.Context, batch []Identifier) ([]Card, error) {
	cards, err := c.fetchBatch(ctx, batch)
	if !errors.Is(err, errRejected) {
		return cards, err
	}
	if len(batch) == 1 {
		return nil, nil
	}
	half := len(batch) / 2
	first, err := c.fetchSplitting(ctx, batch[:half])
	if err != nil {
		return nil, err
	}
	second, err := c.fetchSplitting(ctx, batch[half:])
	if err != nil {
		return nil, err
	}
	return append(first, second...), nil
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

		if err := collectionPacer.wait(ctx); err != nil {
			return nil, err
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

		if resp.StatusCode == http.StatusBadRequest {
			return nil, errRejected
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("scryfall returned status %d", resp.StatusCode)
		}

		var parsed collectionResponse
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return nil, fmt.Errorf("decoding scryfall response: %w", err)
		}

		cards := make([]Card, len(parsed.Data))
		for i, wc := range parsed.Data {
			cards[i] = wc.toCard()
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
