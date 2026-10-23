// Package parallel runs independent lookups concurrently, so a handler pays
// one round trip instead of one per item.
package parallel

import "sync"

// Limit caps concurrent calls to stay under the AWS request quotas.
const Limit = 8

// Map calls f for every item, at most Limit at a time, and returns the
// results in the items' order. The first error wins.
func Map[T, R any](items []T, f func(T) (R, error)) ([]R, error) {
	out := make([]R, len(items))
	errs := make([]error, len(items))
	slots := make(chan struct{}, Limit)
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		slots <- struct{}{}
		go func(i int, it T) {
			defer func() { <-slots; wg.Done() }()
			out[i], errs[i] = f(it)
		}(i, it)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
