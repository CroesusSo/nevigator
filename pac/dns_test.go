package pac

import (
	"fmt"
	"testing"
	"time"
)

func TestDNSCacheLRUEviction(t *testing.T) {
	// Create cache with max 3 entries
	cache := newDNSCache(10*time.Minute, 3)
	cache.enabled = true

	// Insert 4 distinct hosts
	cache.set("host1", []string{"1.1.1.1"})
	cache.set("host2", []string{"2.2.2.2"})
	cache.set("host3", []string{"3.3.3.3"})
	cache.set("host4", []string{"4.4.4.4"}) // should evict host1 (oldest)

	// Verify eviction
	if _, ok := cache.get("host1"); ok {
		t.Errorf("host1 should have been evicted")
	}
	if _, ok := cache.get("host2"); !ok {
		t.Errorf("host2 should still exist")
	}
	if _, ok := cache.get("host3"); !ok {
		t.Errorf("host3 should still exist")
	}
	if _, ok := cache.get("host4"); !ok {
		t.Errorf("host4 should still exist")
	}
}

func TestDNSCacheTTLExpiry(t *testing.T) {
	cache := newDNSCache(100*time.Millisecond, 100)
	cache.enabled = true

	cache.set("host1", []string{"1.1.1.1"})

	// Should be present initially
	if _, ok := cache.get("host1"); !ok {
		t.Errorf("host1 should exist before TTL expiry")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should be gone
	if _, ok := cache.get("host1"); ok {
		t.Errorf("host1 should have expired")
	}
}

func TestDNSCacheGetMovesToMRU(t *testing.T) {
	cache := newDNSCache(10*time.Minute, 3)
	cache.enabled = true

	cache.set("host1", []string{"1.1.1.1"})
	cache.set("host2", []string{"2.2.2.2"})
	cache.set("host3", []string{"3.3.3.3"})

	// Access host1 to make it MRU
	if _, ok := cache.get("host1"); !ok {
		t.Fatal("host1 should exist")
	}

	// Add host4, should evict host2 (oldest)
	cache.set("host4", []string{"4.4.4.4"})

	// Verify eviction: host2 gone, others remain
	if _, ok := cache.get("host2"); ok {
		t.Errorf("host2 should have been evicted as LRU")
	}
	if _, ok := cache.get("host1"); !ok {
		t.Errorf("host1 should still exist (was MRU after access)")
	}
	if _, ok := cache.get("host3"); !ok {
		t.Errorf("host3 should still exist")
	}
	if _, ok := cache.get("host4"); !ok {
		t.Errorf("host4 should exist")
	}
}

func TestDNSCacheUnlimited(t *testing.T) {
	// maxEntries = 0 means unlimited
	cache := newDNSCache(10*time.Minute, 0)
	cache.enabled = true

	// Insert 1000 unique hosts
	for i := 0; i < 1000; i++ {
		host := fmt.Sprintf("host-%d", i)
		cache.set(host, []string{"1.2.3.4"})
	}

	// All should still be retrievable
	for i := 0; i < 1000; i++ {
		host := fmt.Sprintf("host-%d", i)
		if _, ok := cache.get(host); !ok {
			t.Errorf("host %s should still exist in unlimited cache", host)
			return
		}
	}
}
