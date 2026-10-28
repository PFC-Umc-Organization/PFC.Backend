package identity

import (
	"sync"
	"time"
)

// nameTTL bounds how long a renamed account shows its old name. Every
// project page looks names up in Cognito, whose ListUsers quota is a few
// calls per second for the whole pool.
const nameTTL = 5 * time.Minute

// nameCache keeps found names only: a student who signs up shows up on the
// next lookup instead of after the TTL.
type nameCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]cachedName
}

type cachedName struct {
	name    string
	expires time.Time
}

func newNameCache(now func() time.Time) *nameCache {
	return &nameCache{now: now, entries: map[string]cachedName{}}
}

func (c *nameCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !c.now().Before(e.expires) {
		return "", false
	}
	return e.name, true
}

func (c *nameCache) put(key, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cachedName{name: name, expires: c.now().Add(nameTTL)}
}

func (c *nameCache) drop(keys ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.entries, k)
	}
}
