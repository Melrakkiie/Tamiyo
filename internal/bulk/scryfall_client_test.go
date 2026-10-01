package bulk

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

	original := scryfallCollectionURL
	scryfallCollectionURL = server.URL
	t.Cleanup(func() { scryfallCollectionURL = original })
}

func TestScryfallClient_ResolvesKnownIdentifiers(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body scryfallCollectionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Identifiers, 1)
		assert.Equal(t, "znr", body.Identifiers[0].Set)
		assert.Equal(t, "90", body.Identifiers[0].CollectorNumber)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(scryfallCollectionResponse{
			Data: []scryfallCard{
				{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90"},
			},
		})
	})

	client := NewScryfallClient()
	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "ZNR", CollectorNumber: "90"}})

	require.NoError(t, err)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", resolved[resolveKey("ZNR", "90")])
}

func TestScryfallClient_OmitsNotFoundIdentifiers(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(scryfallCollectionResponse{
			NotFound: []scryfallIdentifier{{Set: "xxx", CollectorNumber: "999"}},
		})
	})

	client := NewScryfallClient()
	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "xxx", CollectorNumber: "999"}})

	require.NoError(t, err)
	assert.Empty(t, resolved)
}

func TestScryfallClient_BatchesOver75Identifiers(t *testing.T) {
	var requestSizes []int
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		var body scryfallCollectionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requestSizes = append(requestSizes, len(body.Identifiers))

		data := make([]scryfallCard, len(body.Identifiers))
		for i, id := range body.Identifiers {
			data[i] = scryfallCard{ID: id.Set + "-" + id.CollectorNumber, Set: id.Set, CollectorNumber: id.CollectorNumber}
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(scryfallCollectionResponse{Data: data})
	})

	identifiers := make([]CardIdentifier, 80)
	for i := range identifiers {
		identifiers[i] = CardIdentifier{SetCode: "cmm", CollectorNumber: strconv.Itoa(i)}
	}

	client := NewScryfallClient()
	resolved, err := client.Resolve(context.Background(), identifiers)

	require.NoError(t, err)
	assert.Len(t, resolved, 80)
	require.Len(t, requestSizes, 2)
	assert.Equal(t, 75, requestSizes[0])
	assert.Equal(t, 5, requestSizes[1])
}

func TestScryfallClient_ReturnsErrorOnNonOKStatus(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := NewScryfallClient()
	_, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "znr", CollectorNumber: "90"}})

	require.Error(t, err)
}

func withFastRetries(t *testing.T) {
	t.Helper()
	original := scryfallRetryBaseDelay
	scryfallRetryBaseDelay = time.Millisecond
	t.Cleanup(func() { scryfallRetryBaseDelay = original })
}

func TestScryfallClient_RetriesOn429ThenSucceeds(t *testing.T) {
	withFastRetries(t)

	var requestCount int
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(scryfallCollectionResponse{
			Data: []scryfallCard{{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90"}},
		})
	})

	client := NewScryfallClient()
	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "ZNR", CollectorNumber: "90"}})

	require.NoError(t, err)
	assert.Equal(t, 2, requestCount)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", resolved[resolveKey("ZNR", "90")])
}

func TestScryfallClient_RespectsRetryAfterHeader(t *testing.T) {
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
		_ = json.NewEncoder(w).Encode(scryfallCollectionResponse{
			Data: []scryfallCard{{ID: "11111111-1111-1111-1111-111111111111", Set: "znr", CollectorNumber: "90"}},
		})
	})

	client := NewScryfallClient()
	resolved, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "ZNR", CollectorNumber: "90"}})

	require.NoError(t, err)
	assert.Equal(t, 2, requestCount)
	assert.NotEmpty(t, resolved)
}

func TestScryfallClient_GivesUpAfterMaxRetries(t *testing.T) {
	withFastRetries(t)

	var requestCount int
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusTooManyRequests)
	})

	client := NewScryfallClient()
	_, err := client.Resolve(context.Background(), []CardIdentifier{{SetCode: "znr", CollectorNumber: "90"}})

	require.Error(t, err)
	assert.Equal(t, scryfallMaxRetries+1, requestCount)
}
