package dynamo

import "strings"

// ---------------------------------------------------------------------------
// Escopo
// ---------------------------------------------------------------------------

var scopeSentinels = map[string]bool{"": true, "all": true, "todas": true, "todos": true}

// Verificação
func EmptyScope(id string) bool { return scopeSentinels[strings.ToLower(strings.TrimSpace(id))] }

// Deduplicação
func uniqueKeys(keys []Item) []Item {
	seen := make(map[string]bool, len(keys))
	out := make([]Item, 0, len(keys))
	for _, k := range keys {
		id := String(k, "PK") + "\x00" + String(k, "SK")
		if !seen[id] {
			seen[id] = true
			out = append(out, k)
		}
	}
	return out
}
