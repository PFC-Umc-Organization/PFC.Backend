package parallel

import (
	"context"
	"sync"
)

const Limit = 8

// Paralelismo
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
