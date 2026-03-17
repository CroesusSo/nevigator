package pac

import (
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
}

// dnsCache is a simple in-memory DNS cache.
type dnsCache struct {
	mu       sync.RWMutex
	store    map[string]dnsCacheEntry
	ttl      time.Duration
	enabled  bool          // caching enabled flag
	resolver *net.Resolver // optional custom resolver
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
	if !c.enabled {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.store[host]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.addrs, true
}

func (c *dnsCache) set(host string, addrs []string) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	c.store[host] = dnsCacheEntry{
		addrs:   addrs,
		expires: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
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
