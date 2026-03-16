package pac

import (
	"context"
	"net"
	"sync"
	"time"
)

// dnsCacheEntry holds cached DNS results.
type dnsCacheEntry struct {
	addrs   []string
	expires time.Time
}

// dnsCache is a simple in-memory DNS cache.
type dnsCache struct {
	mu    sync.RWMutex
	store map[string]dnsCacheEntry
	ttl   time.Duration
}

func newDNSCache(ttl time.Duration) *dnsCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &dnsCache{
		store: make(map[string]dnsCacheEntry),
		ttl:   ttl,
	}
}

func (c *dnsCache) get(host string) ([]string, bool) {
	c.mu.RLock()
	entry, ok := c.store[host]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.addrs, true
}

func (c *dnsCache) set(host string, addrs []string) {
	c.mu.Lock()
	c.store[host] = dnsCacheEntry{
		addrs:   addrs,
		expires: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// lookupHost performs a DNS lookup with timeout.
func lookupHost(ctx context.Context, host string) ([]string, error) {
	// Use default resolver which respects system config (e.g., /etc/resolv.conf)
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
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
