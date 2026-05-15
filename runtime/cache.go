package runtime

import (
	"sync"
	"time"
)

type cacheEntry[V any] struct {
	value   V
	expires time.Time
}

type TTLCache[K comparable, V any] struct {
	mu       sync.RWMutex
	entries  map[K]cacheEntry[V]
	ttl      time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewTTLCache[K comparable, V any](ttl time.Duration) *TTLCache[K, V] {
	c := &TTLCache[K, V]{
		entries: make(map[K]cacheEntry[V]),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go c.cleanup()
	return c
}

func (c *TTLCache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		if ok {
			c.mu.Lock()
			delete(c.entries, key)
			c.mu.Unlock()
		}
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (c *TTLCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	c.entries[key] = cacheEntry[V]{value: value, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *TTLCache[K, V]) SetTTL(key K, value V, customTTL time.Duration) {
	c.mu.Lock()
	c.entries[key] = cacheEntry[V]{value: value, expires: time.Now().Add(customTTL)}
	c.mu.Unlock()
}

func (c *TTLCache[K, V]) Delete(key K) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *TTLCache[K, V]) Invalidate(predicate func(K) bool) {
	c.mu.Lock()
	for k := range c.entries {
		if predicate(k) {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}

func (c *TTLCache[K, V]) Purge() {
	c.mu.Lock()
	c.entries = make(map[K]cacheEntry[V])
	c.mu.Unlock()
}

func (c *TTLCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *TTLCache[K, V]) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

func (c *TTLCache[K, V]) cleanup() {
	ticker := time.NewTicker(c.ttl / 2)
	if c.ttl/2 < time.Second {
		ticker.Reset(time.Second)
	}
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.expireEntries()
		}
	}
}

func (c *TTLCache[K, V]) expireEntries() {
	now := time.Now()
	c.mu.Lock()
	for k, entry := range c.entries {
		if now.After(entry.expires) {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}
