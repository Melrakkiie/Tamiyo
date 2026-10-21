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
