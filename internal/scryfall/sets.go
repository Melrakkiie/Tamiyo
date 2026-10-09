package scryfall

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

var setsURL = "https://api.scryfall.com/sets"

const maxSetPages = 10

type setsResponse struct {
	Data []struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"data"`
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
}

func (c *Client) SetNames(ctx context.Context) (map[string]string, error) {
	names := make(map[string]string)
	url := setsURL
	for page := 0; page < maxSetPages && url != ""; page++ {
		parsed, err := c.fetchSets(ctx, url)
		if err != nil {
			return nil, err
		}
		for _, set := range parsed.Data {
			names[strings.ToLower(set.Code)] = set.Name
		}
		url = ""
		if parsed.HasMore {
			url = parsed.NextPage
		}
	}
	return names, nil
}

func (c *Client) fetchSets(ctx context.Context, url string) (setsResponse, error) {
	if err := collectionPacer.wait(ctx); err != nil {
		return setsResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return setsResponse{}, fmt.Errorf("building scryfall request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Tamiyo/1.0 (+https://github.com/Melrakkiie/Tamiyo)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return setsResponse{}, fmt.Errorf("calling scryfall: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return setsResponse{}, fmt.Errorf("scryfall returned status %d", resp.StatusCode)
	}

	var parsed setsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return setsResponse{}, fmt.Errorf("decoding scryfall response: %w", err)
	}
	return parsed, nil
}
