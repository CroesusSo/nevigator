package pac

import (
	"container/list"
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// dnsCacheEntry holds cached DNS results.
type dnsCacheEntry struct {
	addrs   []string
	expires time.Time
	element *list.Element // pointer into lruList
}

// dnsCache is a simple in-memory DNS cache.
type dnsCache struct {
	mu         sync.RWMutex
	store      map[string]dnsCacheEntry
	lruList    *list.List // keys, front = MRU
	maxEntries int        // 0 = unlimited
	ttl        time.Duration
	enabled    bool          // caching enabled flag
	resolver   *net.Resolver // optional custom resolver
}

func newDNSCache(ttl time.Duration, maxEntries int) *dnsCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &dnsCache{
		store:      make(map[string]dnsCacheEntry),
		ttl:        ttl,
		lruList:    list.New(),
		maxEntries: maxEntries,
	}
}

func (c *dnsCache) get(host string) ([]string, bool) {
	if !c.enabled {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.store[host]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	// Move to front of LRU list on hit
	if c.lruList != nil {
		c.mu.Lock()
		if entry, ok := c.store[host]; ok {
			if entry.element != nil {
				c.lruList.MoveToFront(entry.element)
			}
		}
		c.mu.Unlock()
	}
	return entry.addrs, true
}

func (c *dnsCache) set(host string, addrs []string) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	entry := dnsCacheEntry{
		addrs:   addrs,
		expires: now.Add(c.ttl),
	}

	// If already exists, update and move to front
	if existing, ok := c.store[host]; ok {
		entry.element = existing.element
		existing.element.Value = host // update value (should be same)
		c.lruList.MoveToFront(entry.element)
		c.store[host] = entry
		return
	}

	// New entry: create element and add to front
	element := c.lruList.PushFront(host)
	entry.element = element
	c.store[host] = entry

	// Evict if over capacity
	if c.maxEntries > 0 && len(c.store) > c.maxEntries {
		// Remove least recently used (back of list)
		oldest := c.lruList.Back()
		if oldest != nil {
			oldHost := oldest.Value.(string)
			delete(c.store, oldHost)
			c.lruList.Remove(oldest)
		}
	}
}

// lookupHost performs a DNS lookup with timeout.
func (c *dnsCache) lookupHost(ctx context.Context, host string) ([]string, error) {
	// Choose resolver: use custom if configured, else system default
	resolver := c.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	// Filter to IPv4 for simplicity; could include IPv6
	var ipv4Addrs []string
	for _, addr := range addrs {
		if net.ParseIP(addr).To4() != nil {
			ipv4Addrs = append(ipv4Addrs, addr)
		}
	}
	return ipv4Addrs, nil
}

// CreateCustomResolver creates a net.Resolver that uses the specified DNS servers.
// It returns a resolver that tries each server in order until one succeeds.
func CreateCustomResolver(servers []string) *net.Resolver {
	dialer := &net.Dialer{}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		// Try each server in order
		for _, server := range servers {
			conn, err := dialer.DialContext(ctx, network, server)
			if err == nil {
				return conn, nil
			}
			// Continue to next server on failure
		}
		return nil, fmt.Errorf("all custom DNS servers unreachable")
	}
	return &net.Resolver{
		PreferGo: true,
		Dial:     dial,
	}
}
