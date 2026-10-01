package scryfall

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withFakeScryfall(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	original := collectionURL
	collectionURL = server.URL
	t.Cleanup(func() { collectionURL = original })
}

func withFastRetries(t *testing.T) {
	t.Helper()
	original := retryBaseDelay
	retryBaseDelay = time.Millisecond
	t.Cleanup(func() { retryBaseDelay = original })
}

func TestClient_FetchBySetAndCollectorNumber(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body collectionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Identifiers, 1)
		assert.Equal(t, "znr", body.Identifiers[0].Set)
		assert.Equal(t, "90", body.Identifiers[0].CollectorNumber)
		assert.Empty(t, body.Identifiers[0].ID)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90"}},
		})
	})

	client := NewClient()
	cards, err := client.Fetch(context.Background(), []Identifier{{Set: "ZNR", CollectorNumber: "90"}})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", cards[0].ID)
}

func TestClient_FetchByID(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		var body collectionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Identifiers, 1)
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", body.Identifiers[0].ID)
		assert.Empty(t, body.Identifiers[0].Set)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{
				ID: "11111111-1111-1111-1111-111111111111", Name: "Sol Ring",
				ManaCost: "{1}", CMC: 1, TypeLine: "Artifact",
				Colors: []string{}, ColorIdentity: []string{},
				Legalities: map[string]string{"commander": "legal"},
			}},
		})
	})

	client := NewClient()
	cards, err := client.Fetch(context.Background(), []Identifier{{ID: "11111111-1111-1111-1111-111111111111"}})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "Sol Ring", cards[0].Name)
	assert.Equal(t, "Artifact", cards[0].TypeLine)
	assert.Equal(t, 1.0, cards[0].CMC)
	assert.Equal(t, "legal", cards[0].Legalities["commander"])
}

func TestClient_OmitsNotFoundIdentifiers(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			NotFound: []identifier{{Set: "xxx", CollectorNumber: "999"}},
		})
	})

	client := NewClient()
	cards, err := client.Fetch(context.Background(), []Identifier{{Set: "xxx", CollectorNumber: "999"}})

	require.NoError(t, err)
	assert.Empty(t, cards)
}

func TestClient_BatchesOver75Identifiers(t *testing.T) {
	var requestSizes []int
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		var body collectionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requestSizes = append(requestSizes, len(body.Identifiers))

		data := make([]wireCard, len(body.Identifiers))
		for i, id := range body.Identifiers {
			data[i] = wireCard{ID: id.Set + "-" + id.CollectorNumber, Set: id.Set, CollectorNumber: id.CollectorNumber}
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{Data: data})
	})

	identifiers := make([]Identifier, 80)
	for i := range identifiers {
		identifiers[i] = Identifier{Set: "cmm", CollectorNumber: strconv.Itoa(i)}
	}

	client := NewClient()
	cards, err := client.Fetch(context.Background(), identifiers)

	require.NoError(t, err)
	assert.Len(t, cards, 80)
	require.Len(t, requestSizes, 2)
	assert.Equal(t, 75, requestSizes[0])
	assert.Equal(t, 5, requestSizes[1])
}

func TestClient_ReturnsErrorOnNonOKStatus(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := NewClient()
	_, err := client.Fetch(context.Background(), []Identifier{{Set: "znr", CollectorNumber: "90"}})

	require.Error(t, err)
}

func TestClient_RetriesOn429ThenSucceeds(t *testing.T) {
	withFastRetries(t)

	var requestCount int
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90"}},
		})
	})

	client := NewClient()
	cards, err := client.Fetch(context.Background(), []Identifier{{Set: "ZNR", CollectorNumber: "90"}})

	require.NoError(t, err)
	assert.Equal(t, 2, requestCount)
	require.Len(t, cards, 1)
}

func TestClient_RespectsRetryAfterHeader(t *testing.T) {
	withFastRetries(t)

	var requestCount int
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90"}},
		})
	})

	client := NewClient()
	cards, err := client.Fetch(context.Background(), []Identifier{{Set: "ZNR", CollectorNumber: "90"}})

	require.NoError(t, err)
	assert.Equal(t, 2, requestCount)
	assert.NotEmpty(t, cards)
}

func TestClient_GivesUpAfterMaxRetries(t *testing.T) {
	withFastRetries(t)

	var requestCount int
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusTooManyRequests)
	})

	client := NewClient()
	_, err := client.Fetch(context.Background(), []Identifier{{Set: "znr", CollectorNumber: "90"}})

	require.Error(t, err)
	assert.Equal(t, maxRetries+1, requestCount)
}
