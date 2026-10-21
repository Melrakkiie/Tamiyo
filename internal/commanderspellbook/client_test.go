package commanderspellbook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withFakeSpellbook serves handler in place of Commander Spellbook and
// returns a client that calls it, retrying without waiting.
func withFakeSpellbook(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return newClient(server.URL, time.Millisecond)
}

func TestClient_SendsTheDeckAsCardNamesWithATamiyoUserAgent(t *testing.T) {
	var method, userAgent, acceptEncoding, contentType string
	var body map[string][]map[string]string
	client := withFakeSpellbook(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		userAgent = r.Header.Get("User-Agent")
		acceptEncoding = r.Header.Get("Accept-Encoding")
		contentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"results": {"included": []}}`))
	})

	_, err := client.FindCombos(context.Background(), []string{"Niv-Mizzet, Parun"}, []string{"Curiosity", "Bala Ged Recovery // Bala Ged Sanctuary"})

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, method)
	assert.Contains(t, userAgent, "Tamiyo")
	assert.Contains(t, acceptEncoding, "gzip")
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, map[string][]map[string]string{
		"commanders": {{"card": "Niv-Mizzet, Parun"}},
		"main":       {{"card": "Curiosity"}, {"card": "Bala Ged Recovery // Bala Ged Sanctuary"}},
	}, body)
}

func TestClient_KeepsOnlyTheCombosTheDeckIncludes(t *testing.T) {
	client := withFakeSpellbook(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"count": 2,
			"next": null,
			"results": {
				"identity": "UR",
				"included": [{
					"id": "1414-2730",
					"status": "OK",
					"uses": [
						{"card": {"id": 1414, "name": "Niv-Mizzet, Parun", "oracleId": "x"}, "zoneLocations": ["C"]},
						{"card": {"id": 2730, "name": "Curiosity"}, "zoneLocations": ["B"]}
					],
					"produces": [
						{"feature": {"id": 2, "name": "Infinite damage"}, "quantity": 1},
						{"feature": {"id": 3, "name": "Infinite card draw"}, "quantity": 1}
					],
					"description": "Deal damage, draw a card, repeat."
				}],
				"includedByChangingCommanders": [{"id": "9-9", "uses": [], "produces": []}],
				"almostIncluded": [{"id": "8-8", "uses": [], "produces": []}],
				"almostIncludedByAddingColors": [{"id": "7-7", "uses": [], "produces": []}]
			}
		}`))
	})

	combos, err := client.FindCombos(context.Background(), []string{"Niv-Mizzet, Parun"}, []string{"Curiosity"})

	require.NoError(t, err)
	assert.Equal(t, []Combo{{
		ID:      "1414-2730",
		Cards:   []string{"Niv-Mizzet, Parun", "Curiosity"},
		Results: []string{"Infinite damage", "Infinite card draw"},
	}}, combos)
}

func TestClient_ReturnsAnErrorOnAServerError(t *testing.T) {
	var requestCount int
	client := withFakeSpellbook(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"results": {"included": []}}`))
	})

	_, err := client.FindCombos(context.Background(), nil, []string{"Curiosity"})

	require.Error(t, err)
	assert.Equal(t, 1, requestCount)
}

func TestClient_RetriesOnceOn429ThenSucceeds(t *testing.T) {
	var requestCount int
	var bodies []string
	client := withFakeSpellbook(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		encoded, _ := json.Marshal(body)
		bodies = append(bodies, string(encoded))
		if requestCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"results": {"included": [{"id": "1-2", "uses": [], "produces": []}]}}`))
	})

	combos, err := client.FindCombos(context.Background(), nil, []string{"Curiosity"})

	require.NoError(t, err)
	assert.Equal(t, 2, requestCount)
	assert.Equal(t, bodies[0], bodies[1], "the retry resends the same deck")
	require.Len(t, combos, 1)
	assert.Equal(t, "1-2", combos[0].ID)
}

func TestClient_GivesUpAfterASecond429(t *testing.T) {
	var requestCount int
	client := withFakeSpellbook(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := client.FindCombos(context.Background(), nil, []string{"Curiosity"})

	require.Error(t, err)
	assert.Equal(t, 2, requestCount)
}
