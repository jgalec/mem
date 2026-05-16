package runtime

import (
	"sync"
	"sync/atomic"
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
	hits     atomic.Int64
	misses   atomic.Int64
}

type CacheStats struct {
	Size      int
	Hits      int64
	Misses    int64
	HitRate   float64
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
		c.misses.Add(1)
		if ok {
			c.mu.Lock()
			delete(c.entries, key)
			c.mu.Unlock()
		}
		var zero V
		return zero, false
	}
	c.hits.Add(1)
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

func (c *TTLCache[K, V]) Stats() CacheStats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses
	rate := 0.0
	if total > 0 {
		rate = float64(hits) / float64(total)
	}
	return CacheStats{
		Size:    c.Len(),
		Hits:    hits,
		Misses:  misses,
		HitRate: rate,
	}
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
