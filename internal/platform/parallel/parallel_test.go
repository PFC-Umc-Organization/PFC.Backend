package parallel

import (
	"context"
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
	got, err := Map(context.Background(), items, func(_ context.Context, i int) (int, error) {
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
	_, err := Map(context.Background(), []int{1, 2, 3}, func(_ context.Context, i int) (int, error) {
		if i == 2 {
			return 0, boom
		}
		return i, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestMapStopsLaunchingAfterFirstError(t *testing.T) {
	boom := errors.New("boom")
	var started atomic.Int32
	items := make([]int, 1000)
	_, err := Map(context.Background(), items, func(ctx context.Context, _ int) (int, error) {
		if started.Add(1) == 1 {
			return 0, boom
		}
		<-ctx.Done()
		return 0, ctx.Err()
	})
	if !errors.Is(err, boom) {
		t.Fatalf("the first real error must win, got %v", err)
	}
	if n := started.Load(); n >= int32(len(items)) {
		t.Fatalf("%d of %d items started after an error", n, len(items))
	}
}

func TestMapStopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int32
	items := make([]int, 1000)
	_, err := Map(ctx, items, func(ctx context.Context, _ int) (int, error) {
		if started.Add(1) == 3 {
			cancel()
		}
		<-ctx.Done()
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if n := started.Load(); n >= int32(len(items)) {
		t.Fatalf("%d of %d items started after cancel", n, len(items))
	}
}

func TestMapSucceedsWithEmptyInput(t *testing.T) {
	got, err := Map(context.Background(), []int(nil), func(context.Context, int) (int, error) { return 1, nil })
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, err %v", got, err)
	}
}
