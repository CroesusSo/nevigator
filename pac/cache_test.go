package pac

import (
	"sync"
	"testing"
	"time"
)

func TestEvaluatorCacheBasic(t *testing.T) {
	cfg := cacheConfig{
		enabled: true,
		maxSize: 10,
		ttl:     100 * time.Millisecond,
	}
	c := newEvaluatorCache(cfg)
	defer c.Close()

	route1 := Route{Strategy: "TUN"}

	// Set and get
	c.Set("example.com", route1)
	got, ok := c.Get("example.com")
	if !ok || got != route1 {
		t.Fatalf("expected route1, got %v, ok=%v", got, ok)
	}

	// Update existing entry
	route1b := Route{Strategy: "DIRECT"}
	c.Set("example.com", route1b)
	got, ok = c.Get("example.com")
	if !ok || got != route1b {
		t.Fatalf("expected updated route, got %v, ok=%v", got, ok)
	}

	// Miss for unknown host
	_, ok = c.Get("unknown.com")
	if ok {
		t.Error("expected miss for unknown host")
	}

	// Stats
	hits, misses, size, maxSize, hitRate := c.Stats()
	if hits != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
	if size != 1 {
		t.Errorf("size = %d, want 1", size)
	}
	if maxSize != 10 {
		t.Errorf("maxSize = %d, want 10", maxSize)
	}
	if hitRate != 2.0/3.0 {
		t.Errorf("hitRate = %v, want ~0.667", hitRate)
	}
}

func TestEvaluatorCacheTTL(t *testing.T) {
	cfg := cacheConfig{
		enabled: true,
		maxSize: 10,
		ttl:     50 * time.Millisecond,
	}
	c := newEvaluatorCache(cfg)
	defer c.Close()

	route := Route{Strategy: "TUN"}
	c.Set("test.com", route)

	// Immediate hit
	_, ok := c.Get("test.com")
	if !ok {
		t.Fatal("expected immediate hit")
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	_, ok = c.Get("test.com")
	if ok {
		t.Error("expected miss after TTL expiry")
	}
}

func TestEvaluatorCacheLRUEviction(t *testing.T) {
	cfg := cacheConfig{
		enabled: true,
		maxSize: 3,
		ttl:     5 * time.Second,
	}
	c := newEvaluatorCache(cfg)
	defer c.Close()

	// Insert 4 entries, should evict the first one
	c.Set("a", Route{Strategy: "TUN"})
	c.Set("b", Route{Strategy: "TUN"})
	c.Set("c", Route{Strategy: "TUN"})
	c.Set("d", Route{Strategy: "TUN"}) // evicts 'a'

	_, ok := c.Get("a")
	if ok {
		t.Error("expected 'a' to be evicted")
	}
	_, ok = c.Get("b")
	if !ok {
		t.Error("expected 'b' to still be present")
	}
	_, ok = c.Get("c")
	if !ok {
		t.Error("expected 'c' to still be present")
	}
	_, ok = c.Get("d")
	if !ok {
		t.Error("expected 'd' to be present")
	}
}

func TestEvaluatorCacheInvalidateAll(t *testing.T) {
	cfg := cacheConfig{
		enabled: true,
		maxSize: 10,
		ttl:     5 * time.Second,
	}
	c := newEvaluatorCache(cfg)
	defer c.Close()

	c.Set("a", Route{Strategy: "TUN"})
	c.Set("b", Route{Strategy: "SOCKS5", Address: "127.0.0.1:1080"})

	c.InvalidateAll()

	// After invalidation, stats should be reset to zero
	hits, misses, _, _, _ := c.Stats()
	if hits != 0 || misses != 0 {
		t.Errorf("stats immediately after InvalidateAll: hits=%d, misses=%d, want 0,0", hits, misses)
	}

	// Cache should be empty, so Get returns false and increments misses
	_, ok := c.Get("a")
	if ok {
		t.Error("expected cache miss for a after InvalidateAll")
	}
	_, ok = c.Get("b")
	if ok {
		t.Error("expected cache miss for b after InvalidateAll")
	}

	// Now stats should reflect the two misses
	hits, misses, _, _, _ = c.Stats()
	if hits != 0 {
		t.Errorf("hits after two misses = %d, want 0", hits)
	}
	if misses != 2 {
		t.Errorf("misses after two gets = %d, want 2", misses)
	}
}

func TestEvaluatorCacheConcurrent(t *testing.T) {
	cfg := cacheConfig{
		enabled: true,
		maxSize: 1000,
		ttl:     5 * time.Second,
	}
	c := newEvaluatorCache(cfg)
	defer c.Close()

	var wg sync.WaitGroup
	numGoroutines := 50
	operationsPerGoroutine := 200

	// Mixed concurrent reads and writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				host := string(rune('a')+rune((idx*j)%26)) + ".com"
				route := Route{Strategy: "TUN"}
				c.Set(host, route)
				c.Get(host)
			}
		}(i)
	}

	wg.Wait()

	// Should not have panicked or data race
	hits, misses, size, _, _ := c.Stats()
	t.Logf("hits=%d, misses=%d, size=%d", hits, misses, size)
	// If no panic, test passes (races detected by -race)
}

func TestEvaluatorCacheDisabled(t *testing.T) {
	cfg := cacheConfig{
		enabled: false,
		maxSize: 10,
		ttl:     5 * time.Second,
	}
	c := newEvaluatorCache(cfg)
	if c != nil {
		t.Fatalf("expected nil cache when disabled, got %v", c)
	}
}
