package httpx

import (
	"context"
	"sync"
	"time"
)

type Limiter struct {
	mu sync.Mutex

	next time.Time // the earliest free slot in the gap
	gap  time.Duration
}

func NewLimiter(gap time.Duration) *Limiter {
	return &Limiter{gap: gap}
}

func (l *Limiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	untilSlot := l.next.Sub(now)
	l.next = l.next.Add(l.gap)
	l.mu.Unlock()

	err := Sleep(ctx, untilSlot)
	if err != nil {
		l.mu.Lock()
		l.next = l.next.Add(-l.gap)
		l.mu.Unlock()
	}
	return err
}

func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
