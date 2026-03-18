package pac

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

// cacheEntry represents a single cache entry.
type cacheEntry struct {
	route     Route
	expiresAt time.Time
	element   *list.Element // pointer to the element in the LRU list
}

// EvaluatorCache implements an LRU cache with TTL for PAC evaluation results.
type EvaluatorCache struct {
	mu              sync.RWMutex
	entries         map[string]*cacheEntry
	lruList         *list.List // list of keys, most recent at front
	maxSize         int
	ttl             time.Duration
	hits            atomic.Int64
	misses          atomic.Int64
	stopChan        chan struct{}
	cleanupInterval time.Duration
}

// cacheConfig holds configuration for the cache.
type cacheConfig struct {
	enabled         bool
	maxSize         int
	ttl             time.Duration
	cleanupInterval time.Duration
}

func newEvaluatorCache(cfg cacheConfig) *EvaluatorCache {
	if !cfg.enabled || cfg.maxSize <= 0 || cfg.ttl <= 0 {
		return nil
	}
	c := &EvaluatorCache{
		entries:         make(map[string]*cacheEntry),
		lruList:         list.New(),
		maxSize:         cfg.maxSize,
		ttl:             cfg.ttl,
		stopChan:        make(chan struct{}),
		cleanupInterval: cfg.cleanupInterval,
	}
	if c.cleanupInterval <= 0 {
		c.cleanupInterval = time.Minute
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves a cached route for the given host. Returns true if hit.
func (c *EvaluatorCache) Get(host string) (Route, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[host]
	if !ok {
		c.misses.Add(1)
		return Route{}, false
	}
	if time.Now().After(entry.expiresAt) {
		// Expired, remove it
		c.removeLocked(host, entry)
		c.misses.Add(1)
		return Route{}, false
	}
	// Move to front (most recent)
	c.lruList.MoveToFront(entry.element)
	c.hits.Add(1)
	return entry.route, true
}

// Set stores a route for the given host. If over capacity, evicts LRU.
func (c *EvaluatorCache) Set(host string, route Route) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiresAt := now.Add(c.ttl)

	// Update existing or insert new
	if existing, ok := c.entries[host]; ok {
		existing.route = route
		existing.expiresAt = expiresAt
		c.lruList.MoveToFront(existing.element)
	} else {
		element := c.lruList.PushFront(host)
		c.entries[host] = &cacheEntry{
			route:     route,
			expiresAt: expiresAt,
			element:   element,
		}
	}

	// Evict if over capacity
	if len(c.entries) > c.maxSize {
		c.evictLRU()
	}
}

// evictLRU removes the least recently used entry.
func (c *EvaluatorCache) evictLRU() {
	oldest := c.lruList.Back()
	if oldest == nil {
		return
	}
	host := oldest.Value.(string)
	c.removeLocked(host, c.entries[host])
}

// removeLocked removes an entry without locking (caller holds lock).
func (c *EvaluatorCache) removeLocked(host string, entry *cacheEntry) {
	c.lruList.Remove(entry.element)
	delete(c.entries, host)
}

// InvalidateAll clears all entries and resets statistics.
func (c *EvaluatorCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry)
	c.lruList = list.New()
	c.hits.Store(0)
	c.misses.Store(0)
}

// Stats returns current cache statistics.
func (c *EvaluatorCache) Stats() (hits, misses, size, maxSize int, hitRate float64) {
	hits = int(c.hits.Load())
	misses = int(c.misses.Load())
	size = len(c.entries)
	maxSize = c.maxSize
	if hits+misses > 0 {
		hitRate = float64(hits) / float64(hits+misses)
	}
	return
}

// Close stops the background cleanup goroutine.
func (c *EvaluatorCache) Close() error {
	close(c.stopChan)
	return nil
}

// cleanupLoop periodically removes expired entries.
func (c *EvaluatorCache) cleanupLoop() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopChan:
			return
		}
	}
}

// cleanup removes expired entries.
func (c *EvaluatorCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []string
	for host, entry := range c.entries {
		if now.After(entry.expiresAt) {
			toRemove = append(toRemove, host)
		}
	}
	for _, host := range toRemove {
		c.removeLocked(host, c.entries[host])
	}
}
