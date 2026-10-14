package exchangerate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var frankfurterURL = "https://api.frankfurter.dev/v1/latest"

type FrankfurterClient struct {
	httpClient *http.Client
}

func NewFrankfurterClient() *FrankfurterClient {
	return &FrankfurterClient{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

type frankfurterResponse struct {
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

func (c *FrankfurterClient) Fetch(ctx context.Context, currency string) (Rate, error) {
	symbol := strings.ToUpper(currency)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, frankfurterURL+"?base=EUR&symbols="+symbol, nil)
	if err != nil {
		return Rate{}, fmt.Errorf("building frankfurter request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Rate{}, fmt.Errorf("calling frankfurter: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return Rate{}, fmt.Errorf("frankfurter returned status %d: %s", resp.StatusCode, bodyStart(resp.Body))
	}

	var body frankfurterResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Rate{}, fmt.Errorf("decoding frankfurter response: %w", err)
	}
	rate := body.Rates[symbol]
	if rate <= 0 {
		return Rate{}, fmt.Errorf("frankfurter has no rate for %s", currency)
	}
	date, err := time.Parse(time.DateOnly, body.Date)
	if err != nil {
		return Rate{}, fmt.Errorf("parsing frankfurter date %q: %w", body.Date, err)
	}
	return Rate{Currency: currency, PerEUR: rate, Date: date}, nil
}
