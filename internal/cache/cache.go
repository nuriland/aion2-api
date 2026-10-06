package cache

import (
	"context"
	"sync"
	"time"
)

type Value[T any] struct {
	TTL, Retry time.Duration

	mu       sync.Mutex
	value    T
	ok       bool
	nextLoad time.Time
	triedAt  time.Time
	loading  chan struct{}
}

func (v *Value[T]) Get(ctx context.Context, load func(context.Context) (T, error)) (T, error) {
	for {
		v.mu.Lock()

		if v.ok && (v.loading != nil || time.Now().Before(v.nextLoad)) {
			defer v.mu.Unlock()
			return v.value, nil
		}
		loading := v.loading
		if loading == nil {
			v.loading = make(chan struct{})
			v.mu.Unlock()
			return v.fill(ctx, load)
		}
		v.mu.Unlock()

		select {
		case <-loading:
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
}

func (v *Value[T]) fill(ctx context.Context, load func(context.Context) (T, error)) (value T, err error) {
	returned := false
	defer func() {
		v.mu.Lock()
		defer v.mu.Unlock()

		now := time.Now()
		switch {
		case !returned:
		case err == nil:
			v.value, v.ok, v.nextLoad = value, true, now.Add(v.TTL)
		case v.ok:
			v.nextLoad = now.Add(v.Retry)
		}
		v.triedAt = now
		close(v.loading)
		v.loading = nil
	}()
	value, err = load(ctx)
	returned = true
	return value, err
}

func (v *Value[T]) Expire() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.ok || time.Since(v.triedAt) < v.Retry {
		return false
	}
	v.nextLoad = time.Now()
	return true
}

func (v *Value[T]) Last() (T, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.value, v.ok
}
