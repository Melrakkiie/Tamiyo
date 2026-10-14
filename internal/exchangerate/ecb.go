package exchangerate

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ecbURL = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"

const (
	userAgent    = "Tamiyo/1.0 (+https://github.com/Melrakkiie/Tamiyo)"
	bodyStartMax = 200
)

type ECBClient struct {
	httpClient *http.Client
}

func NewECBClient() *ECBClient {
	return &ECBClient{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

type ecbEnvelope struct {
	Days []struct {
		Time  string `xml:"time,attr"`
		Rates []struct {
			Currency string  `xml:"currency,attr"`
			Rate     float64 `xml:"rate,attr"`
		} `xml:"Cube"`
	} `xml:"Cube>Cube"`
}

func (c *ECBClient) Fetch(ctx context.Context, currency string) (Rate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ecbURL, nil)
	if err != nil {
		return Rate{}, fmt.Errorf("building ecb request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Rate{}, fmt.Errorf("calling ecb: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return Rate{}, fmt.Errorf("ecb returned status %d: %s", resp.StatusCode, bodyStart(resp.Body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Rate{}, fmt.Errorf("reading ecb response: %w", err)
	}
	var envelope ecbEnvelope
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&envelope); err != nil {
		return Rate{}, fmt.Errorf("decoding ecb response: %w: %s", err, truncate(body))
	}
	for _, day := range envelope.Days {
		date, err := time.Parse(time.DateOnly, day.Time)
		if err != nil {
			return Rate{}, fmt.Errorf("parsing ecb date %q: %w", day.Time, err)
		}
		for _, r := range day.Rates {
			if strings.EqualFold(r.Currency, currency) && r.Rate > 0 {
				return Rate{Currency: currency, PerEUR: r.Rate, Date: date}, nil
			}
		}
	}
	return Rate{}, fmt.Errorf("ecb response has no rate for %s (%d days): %s", currency, len(envelope.Days), truncate(body))
}

func bodyStart(r io.Reader) string {
	body, _ := io.ReadAll(io.LimitReader(r, bodyStartMax))
	return truncate(body)
}

func truncate(body []byte) string {
	if len(body) > bodyStartMax {
		body = body[:bodyStartMax]
	}
	return strings.Join(strings.Fields(string(body)), " ")
}
