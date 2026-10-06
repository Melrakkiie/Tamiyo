package scryfall

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRequestInterval(t *testing.T, interval time.Duration) {
	t.Helper()
	original := minRequestInterval
	minRequestInterval = interval
	t.Cleanup(func() { minRequestInterval = original })
}

func TestPacer_SpacesConsecutiveRequests(t *testing.T) {
	withRequestInterval(t, 20*time.Millisecond)
	p := &pacer{}

	start := time.Now()
	for i := 0; i < 3; i++ {
		require.NoError(t, p.wait(context.Background()))
	}

	assert.GreaterOrEqual(t, time.Since(start), 40*time.Millisecond)
}

func TestPacer_SpacesConcurrentRequests(t *testing.T) {
	withRequestInterval(t, 20*time.Millisecond)
	p := &pacer{}

	var mu sync.Mutex
	var times []time.Time
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, p.wait(context.Background()))
			mu.Lock()
			times = append(times, time.Now())
			mu.Unlock()
		}()
	}
	wg.Wait()

	first, last := times[0], times[0]
	for _, at := range times {
		if at.Before(first) {
			first = at
		}
		if at.After(last) {
			last = at
		}
	}
	assert.GreaterOrEqual(t, last.Sub(first), 55*time.Millisecond)
}

func TestPacer_StopsWaitingWhenContextIsCancelled(t *testing.T) {
	withRequestInterval(t, time.Hour)
	p := &pacer{}
	require.NoError(t, p.wait(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.ErrorIs(t, p.wait(ctx), context.Canceled)
}
