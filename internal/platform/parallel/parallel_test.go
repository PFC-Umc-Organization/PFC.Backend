package parallel

import (
	"errors"
	"slices"
	"sync/atomic"
	"testing"
)

func TestMapKeepsOrderAndLimit(t *testing.T) {
	var running, peak atomic.Int32
	items := make([]int, 40)
	for i := range items {
		items[i] = i
	}
	got, err := Map(items, func(i int) (int, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		defer running.Add(-1)
		return i * 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := make([]int, 40)
	for i := range want {
		want[i] = i * 2
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
	if peak.Load() > Limit {
		t.Fatalf("%d calls at once, limit is %d", peak.Load(), Limit)
	}
}

func TestMapReturnsError(t *testing.T) {
	boom := errors.New("boom")
	_, err := Map([]int{1, 2, 3}, func(i int) (int, error) {
		if i == 2 {
			return 0, boom
		}
		return i, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
