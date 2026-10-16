package dynamo

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestFailedAt(t *testing.T) {
	err := fmt.Errorf("save: %w", &ConditionError{Failed: []int{0, 2}})
	for i, want := range []bool{true, false, true, false} {
		if got := FailedAt(err, i); got != want {
			t.Errorf("FailedAt(err, %d) = %v, want %v", i, got, want)
		}
	}
	if FailedAt(errors.New("other"), 0) || FailedAt(nil, 0) {
		t.Error("a non-condition error must not count as a failed item")
	}
}

func TestRetryUnprocessed(t *testing.T) {
	calls := 0
	err := retryUnprocessed(context.Background(), func() (int, error) {
		calls++
		return 3 - calls, nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("calls = %d, err = %v", calls, err)
	}

	calls = 0
	err = retryUnprocessed(context.Background(), func() (int, error) { calls++; return 1, nil })
	if err == nil || calls != batchAttempts {
		t.Fatalf("calls = %d, err = %v", calls, err)
	}
}

func TestString(t *testing.T) {
	it := Item{"PK": S("CURSO#1"), "n": N(3)}
	if String(it, "PK") != "CURSO#1" || String(it, "n") != "" || String(it, "x") != "" {
		t.Fatal("String mismatch")
	}
}
