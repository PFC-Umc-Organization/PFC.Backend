package dynamo

import "testing"

// ---------------------------------------------------------------------------
// Escopo
// ---------------------------------------------------------------------------

// Sentinelas
func TestEmptyScope(t *testing.T) {
	for id, want := range map[string]bool{"": true, "  ": true, "all": true, "Todas": true, "TODOS": true, "abc": false, "p1": false} {
		if got := EmptyScope(id); got != want {
			t.Errorf("%q: got %v", id, got)
		}
	}
}

// Duplicatas
func TestUniqueKeys(t *testing.T) {
	keys := []Item{Key("A#1", "P"), Key("A#1", "P"), Key("A#1", "Q"), Key("A#2", "P")}
	if got := uniqueKeys(keys); len(got) != 3 {
		t.Errorf("got %d keys", len(got))
	}
}
