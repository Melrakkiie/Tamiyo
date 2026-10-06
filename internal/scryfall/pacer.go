package scryfall

import (
	"context"
	"sync"
	"time"
)

var minRequestInterval = 550 * time.Millisecond

type pacer struct {
	mu   sync.Mutex
	next time.Time
}

var collectionPacer = &pacer{}

func (p *pacer) wait(ctx context.Context) error {
	p.mu.Lock()
	now := time.Now()
	slot := p.next
	if slot.Before(now) {
		slot = now
	}
	p.next = slot.Add(minRequestInterval)
	p.mu.Unlock()

	delay := time.Until(slot)
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
