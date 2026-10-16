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

	withRequestInterval(t, 0)
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

func TestClient_FetchByName(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		var raw map[string][]map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		require.Len(t, raw["identifiers"], 1)
		assert.Equal(t, map[string]string{"name": "Lightning Bolt"}, raw["identifiers"][0])

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{ID: "11111111-1111-1111-1111-111111111111", Name: "Lightning Bolt", Set: "2xm", CollectorNumber: "129"}},
		})
	})

	client := NewClient()
	cards, err := client.Fetch(context.Background(), []Identifier{{Name: "Lightning Bolt"}})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "2xm", cards[0].Set)
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
				ManaCost: "{1}", CMC: new(1.0), TypeLine: "Artifact",
				OracleText: "{T}: Add {C}{C}.", Rarity: "uncommon", Keywords: []string{},
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
	assert.Equal(t, "{T}: Add {C}{C}.", cards[0].OracleText)
	assert.Equal(t, "uncommon", cards[0].Rarity)
	assert.Equal(t, []string{}, cards[0].Keywords)
}

func TestClient_JoinsTheOracleTextsOfFaces(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{
				ID: "22222222-2222-2222-2222-222222222222", Name: "Fire // Ice", Keywords: []string{"Flash"},
				CardFaces: []wireCardFace{{OracleText: "Fire deals 2 damage."}, {OracleText: "Tap target permanent."}},
			}},
		})
	})

	cards, err := NewClient().Fetch(context.Background(), []Identifier{{ID: "22222222-2222-2222-2222-222222222222"}})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "Fire deals 2 damage.\n//\nTap target permanent.", cards[0].OracleText)
	assert.Equal(t, []string{"Flash"}, cards[0].Keywords)
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

func TestClient_TakesColorsFromFacesForDoubleFacedCards(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{
				ID: "22222222-2222-2222-2222-222222222222", Name: "Delver of Secrets // Insectile Aberration",
				TypeLine:  "Creature — Human Wizard // Creature — Human Insect",
				CardFaces: []wireCardFace{{Colors: []string{"U"}}, {Colors: []string{"U"}}},
			}},
		})
	})

	cards, err := NewClient().Fetch(context.Background(), []Identifier{{ID: "22222222-2222-2222-2222-222222222222"}})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, []string{"U"}, cards[0].Colors)
}

func TestClient_TakesManaValueAndTypeFromFacesForReversibleCards(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{
				ID: "22222222-2222-2222-2222-222222222222", Name: "Marang River Regent // Coil and Catch // Marang River Regent", Layout: "reversible_card",
				CardFaces: []wireCardFace{
					{ManaCost: "{4}{U}{U}", CMC: new(6.0), TypeLine: "Creature — Dragon"},
					{ManaCost: "{3}{U}", CMC: new(6.0), TypeLine: "Instant — Omen"},
				},
			}},
		})
	})

	cards, err := NewClient().Fetch(context.Background(), []Identifier{{ID: "22222222-2222-2222-2222-222222222222"}})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "Marang River Regent // Coil and Catch", cards[0].Name)
	assert.Equal(t, 6.0, cards[0].CMC)
	assert.Equal(t, "{4}{U}{U}", cards[0].ManaCost)
	assert.Equal(t, "Creature — Dragon // Instant — Omen", cards[0].TypeLine)
}

func TestClient_NamesReversibleCardsOnce(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{
				{ID: "22222222-2222-2222-2222-222222222222", Name: "Clarion Conqueror // Clarion Conqueror", Layout: "reversible_card"},
				{ID: "33333333-3333-3333-3333-333333333333", Name: "Fire // Ice", Layout: "split"},
			},
		})
	})

	cards, err := NewClient().Fetch(context.Background(), []Identifier{
		{ID: "22222222-2222-2222-2222-222222222222"}, {ID: "33333333-3333-3333-3333-333333333333"},
	})

	require.NoError(t, err)
	require.Len(t, cards, 2)
	assert.Equal(t, "Clarion Conqueror", cards[0].Name)
	assert.Equal(t, "Fire // Ice", cards[1].Name)
}

func TestClient_KeepsTopLevelZeroManaValueOverFaces(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{{
				ID: "22222222-2222-2222-2222-222222222222", CMC: new(0.0), TypeLine: "Land // Land",
				CardFaces: []wireCardFace{{CMC: new(3.0)}, {}},
			}},
		})
	})

	cards, err := NewClient().Fetch(context.Background(), []Identifier{{ID: "22222222-2222-2222-2222-222222222222"}})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, 0.0, cards[0].CMC)
	assert.Equal(t, "Land // Land", cards[0].TypeLine)
}

func TestClient_GathersPowersAndToughnessesFromEveryFace(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(collectionResponse{
			Data: []wireCard{
				{
					ID:        "22222222-2222-2222-2222-222222222222",
					CardFaces: []wireCardFace{{Power: "1", Toughness: "1"}, {Power: "3", Toughness: "1"}},
				},
				{ID: "33333333-3333-3333-3333-333333333333", Power: "*", Toughness: "1+*"},
				{ID: "44444444-4444-4444-4444-444444444444"},
			},
		})
	})

	cards, err := NewClient().Fetch(context.Background(), []Identifier{
		{ID: "22222222-2222-2222-2222-222222222222"}, {ID: "33333333-3333-3333-3333-333333333333"}, {ID: "44444444-4444-4444-4444-444444444444"},
	})

	require.NoError(t, err)
	require.Len(t, cards, 3)
	assert.Equal(t, []string{"1", "3"}, cards[0].Powers)
	assert.Equal(t, []string{"1"}, cards[0].Toughnesses)
	assert.Equal(t, []string{"*"}, cards[1].Powers)
	assert.Equal(t, []string{"1+*"}, cards[1].Toughnesses)
	assert.Equal(t, []string{}, cards[2].Powers)
	assert.Equal(t, []string{}, cards[2].Toughnesses)
}

func TestClient_ParsesPrices(t *testing.T) {
	withFakeScryfall(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"11111111-1111-1111-1111-111111111111","prices":{"eur":"1.23","eur_foil":null,"usd":"2.50","usd_foil":"bad","usd_etched":"9.75"}}]}`))
	})

	cards, err := NewClient().Fetch(context.Background(), []Identifier{{ID: "11111111-1111-1111-1111-111111111111"}})

	require.NoError(t, err)
	require.Len(t, cards, 1)
	prices := cards[0].Prices
	require.NotNil(t, prices.EUR)
	assert.Equal(t, 1.23, *prices.EUR)
	assert.Nil(t, prices.EURFoil)
	require.NotNil(t, prices.USD)
	assert.Equal(t, 2.5, *prices.USD)
	assert.Nil(t, prices.USDFoil)
	require.NotNil(t, prices.USDEtched)
	assert.Equal(t, 9.75, *prices.USDEtched)
}

func TestPrices_For(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	tests := []struct {
		name      string
		prices    Prices
		foil      bool
		usdPerEUR float64
		wantEUR   float64
		wantUSD   float64
	}{
		{name: "regular", prices: Prices{EUR: price(1), EURFoil: price(3), USD: price(2), USDFoil: price(4)}, wantEUR: 1, wantUSD: 2},
		{name: "foil", prices: Prices{EUR: price(1), EURFoil: price(3), USD: price(2), USDFoil: price(4)}, foil: true, wantEUR: 3, wantUSD: 4},
		{name: "regular falls back to foil", prices: Prices{EURFoil: price(3), USDFoil: price(4)}, wantEUR: 3, wantUSD: 4},
		{name: "foil falls back to regular", prices: Prices{EUR: price(1), USD: price(2)}, foil: true, wantEUR: 1, wantUSD: 2},
		{name: "euros converted from dollars", prices: Prices{USD: price(5)}, usdPerEUR: 1.25, wantEUR: 4, wantUSD: 5},
		{name: "converted dollars of the same finish before euros of the other", prices: Prices{EUR: price(1), USDFoil: price(10)}, foil: true, usdPerEUR: 1.25, wantEUR: 8, wantUSD: 10},
		{name: "etched dollars stand for the foil price", prices: Prices{USDEtched: price(10)}, foil: true, usdPerEUR: 1.25, wantEUR: 8, wantUSD: 10},
		{name: "foil dollars before etched dollars", prices: Prices{USDFoil: price(4), USDEtched: price(10)}, foil: true, wantUSD: 4},
		{name: "regular falls back to etched", prices: Prices{USDEtched: price(10)}, usdPerEUR: 1.25, wantEUR: 8, wantUSD: 10},
		{name: "no conversion without a rate", prices: Prices{USD: price(5)}, wantUSD: 5},
		{name: "no price", foil: true, usdPerEUR: 1.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eur, usd := tt.prices.For(tt.foil, tt.usdPerEUR)
			assert.Equal(t, tt.wantEUR, eur)
			assert.Equal(t, tt.wantUSD, usd)
		})
	}
}
