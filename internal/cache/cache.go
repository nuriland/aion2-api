package cache

import (
	"context"
	"sync"
	"time"
)

type Value[T any] struct {
	TTL time.Duration

	mu    sync.Mutex
	value T

	loadedAt time.Time     // zero until a load succeeds
	loading  chan struct{} // closed when the load in flight ends
}

func (v *Value[T]) Get(ctx context.Context, load func(context.Context) (T, error)) (T, error) {
	for {
		v.mu.Lock()
		if !v.loadedAt.IsZero() && v.age() < v.TTL {
			defer v.mu.Unlock()
			return v.value, nil
		}
		inFlight := v.loading
		if inFlight == nil {
			v.loading = make(chan struct{})
			v.mu.Unlock()
			return v.fill(ctx, load)
		}
		v.mu.Unlock()

		select {
		case <-inFlight: // go round: it is loaded, or it is our turn
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
}

// The release is deferred so a panicking loader cannot strand the waiters.
func (v *Value[T]) fill(ctx context.Context, load func(context.Context) (T, error)) (value T, err error) {
	var loaded = false
	defer func() {
		v.mu.Lock()
		if loaded {
			v.value, v.loadedAt = value, time.Now()
		}
		close(v.loading)
		v.loading = nil
		v.mu.Unlock()
	}()

	value, err = load(ctx)
	loaded = err == nil
	return value, err
}

// Expire drops a value older than minAge and reports whether it did. minAge
// stops a key that will never be known from forcing a reload on every lookup.
func (v *Value[T]) Expire(minAge time.Duration) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.loadedAt.IsZero() || v.age() < minAge {
		return false
	}
	v.loadedAt = time.Time{}
	return true
}

func (v *Value[T]) age() time.Duration { return time.Since(v.loadedAt) }
