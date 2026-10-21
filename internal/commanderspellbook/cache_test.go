package commanderspellbook

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

const oneComboBody = `{"results": {"included": [{"id": "1-2", "uses": [{"card": {"name": "A"}}], "produces": [{"feature": {"name": "Infinite mana"}}]}]}}`

// cachedClient returns a client over a fake Spellbook that counts its calls,
// with a controllable clock and the given cache bound.
func cachedClient(t *testing.T, maxEntries int, handler http.HandlerFunc) (*Client, *fakeClock, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	client := withFakeSpellbook(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	})
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	client.cache = newComboCache(clock.Now, maxEntries)
	return client, clock, &calls
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(oneComboBody))
}

func TestClient_AnswersTheSameListInAnotherOrderFromCache(t *testing.T) {
	client, _, calls := cachedClient(t, 10, okHandler)
	ctx := context.Background()

	first, err := client.FindCombos(ctx, []string{"Zur", "Atraxa"}, []string{"Curiosity", "Bolt", "Ponder"})
	require.NoError(t, err)
	second, err := client.FindCombos(ctx, []string{"Atraxa", "Zur"}, []string{"Ponder", "Curiosity", "Bolt"})
	require.NoError(t, err)

	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, first, second)
}

func TestClient_KeepsCommandersAndMainApartInTheKey(t *testing.T) {
	client, _, calls := cachedClient(t, 10, okHandler)
	ctx := context.Background()

	_, err := client.FindCombos(ctx, []string{"A"}, []string{"B"})
	require.NoError(t, err)
	_, err = client.FindCombos(ctx, []string{"B"}, []string{"A"})
	require.NoError(t, err)

	assert.Equal(t, int32(2), calls.Load())
}

func TestClient_AsksAgainOnceAnEntryIs24HoursOld(t *testing.T) {
	client, clock, calls := cachedClient(t, 10, okHandler)
	ctx := context.Background()

	_, err := client.FindCombos(ctx, nil, []string{"A"})
	require.NoError(t, err)
	clock.Advance(24*time.Hour - time.Second)
	_, err = client.FindCombos(ctx, nil, []string{"A"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())

	clock.Advance(time.Second)
	_, err = client.FindCombos(ctx, nil, []string{"A"})
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}

func TestClient_EvictsTheOldestEntryPastTheBound(t *testing.T) {
	client, clock, calls := cachedClient(t, 2, okHandler)
	ctx := context.Background()

	for _, card := range []string{"A", "B", "C"} {
		_, err := client.FindCombos(ctx, nil, []string{card})
		require.NoError(t, err)
		clock.Advance(time.Minute)
	}
	require.Equal(t, int32(3), calls.Load())

	_, err := client.FindCombos(ctx, nil, []string{"B"})
	require.NoError(t, err)
	_, err = client.FindCombos(ctx, nil, []string{"C"})
	require.NoError(t, err)
	assert.Equal(t, int32(3), calls.Load(), "B and C are still cached")

	_, err = client.FindCombos(ctx, nil, []string{"A"})
	require.NoError(t, err)
	assert.Equal(t, int32(4), calls.Load(), "A, the oldest, was evicted")
}

func TestClient_DoesNotCacheErrors(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	client, _, calls := cachedClient(t, 10, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		okHandler(w, r)
	})
	ctx := context.Background()

	_, err := client.FindCombos(ctx, nil, []string{"A"})
	require.Error(t, err)

	fail.Store(false)
	combos, err := client.FindCombos(ctx, nil, []string{"A"})
	require.NoError(t, err)
	assert.Len(t, combos, 1)
	assert.Equal(t, int32(2), calls.Load())
}

func TestClient_CachedResultsCannotBeAlteredByCallers(t *testing.T) {
	client, _, _ := cachedClient(t, 10, okHandler)
	ctx := context.Background()

	first, err := client.FindCombos(ctx, nil, []string{"A"})
	require.NoError(t, err)
	first[0].Cards[0] = "mutated"

	second, err := client.FindCombos(ctx, nil, []string{"A"})
	require.NoError(t, err)
	assert.Equal(t, []string{"A"}, second[0].Cards)
}

func TestClient_IsSafeForConcurrentUse(t *testing.T) {
	client, _, _ := cachedClient(t, 3, okHandler)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			card := string(rune('A' + i%5))
			_, err := client.FindCombos(ctx, nil, []string{card})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}
