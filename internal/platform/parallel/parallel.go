// Package parallel runs independent lookups concurrently, so a handler pays
// one round trip instead of one per item.
package parallel

import (
	"context"
	"sync"
)

// Limit caps concurrent calls to stay under the AWS request quotas.
const Limit = 8

// Map calls f for every item, at most Limit at a time, and returns the
// results in the items' order. The first error wins: it cancels the context
// handed to f, and no further item is started. When ctx itself ends first,
// Map stops launching and returns ctx's error.
func Map[T, R any](ctx context.Context, items []T, f func(context.Context, T) (R, error)) ([]R, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	out := make([]R, len(items))
	slots := make(chan struct{}, Limit)
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
		skipped  bool
	)
launch:
	for i, it := range items {
		select {
		case slots <- struct{}{}:
		case <-runCtx.Done():
			skipped = true
			break launch
		}
		if runCtx.Err() != nil {
			<-slots
			skipped = true
			break
		}
		wg.Add(1)
		go func(i int, it T) {
			defer func() { <-slots; wg.Done() }()
			r, err := f(runCtx, it)
			if err != nil {
				once.Do(func() { firstErr = err; cancel() })
				return
			}
			out[i] = r
		}(i, it)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if skipped {
		return nil, ctx.Err()
	}
	return out, nil
}
