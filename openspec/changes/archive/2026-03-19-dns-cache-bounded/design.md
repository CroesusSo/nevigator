# Problem

1. **Unbounded DNS cache**: `dnsCache.store` is a map with no size limit; only TTL expires entries. In scenarios with many unique hosts, memory usage can grow without bound until TTL expires (5m default).
2. **Non-configurable DNS TTL**: The 5-minute TTL is hardcoded; users cannot adjust based on their needs.
3. **Missing config defaults**: Auto-generated config does not include DNS cache parameters, making them less discoverable.

# Solution

## 1. Add Config Parameters

In `pac/config.go` → `EvaluatorConfig`:

```go
type EvaluatorConfig struct {
    // ...
    DNSMaxEntries int           // new; default 100 (0 = unlimited)
    DNSTTL        time.Duration // new; default 5 * time.Minute
    // ...
}
```

## 2. Extend dnsCache with LRU

Modify `pac/dns.go`:

```go
type dnsCache struct {
    mu         sync.RWMutex
    entries    map[string]dnsCacheEntry
    lruList    *list.List        // keys, front = MRU
    maxEntries int               // 0 = unlimited
    ttl        time.Duration
    // ...
}

func newDNSCache(ttl time.Duration, maxEntries int) *dnsCache {
    if ttl == 0 {
        ttl = 5 * time.Minute
    }
    c := &dnsCache{
        entries: make(map[string]dnsCacheEntry),
        ttl:     ttl,
    }
    if maxEntries <= 0 {
        c.maxEntries = 100 // default
    } else {
        c.maxEntries = maxEntries
    }
    return c
}
```

- `get(host)`: On hit, move host to front of `lruList`.
- `set(host, addrs)`: Insert/update entry; push host to front of `lruList`. If `maxEntries > 0 && len(entries) > maxEntries`, evict the LRU (back of list).
- No separate cleanup goroutine needed; TTL checks happen on access and eviction keeps size bounded.

**Also update `NewEvaluator`** (in `pac/evaluator.go`) to call:
```go
e.dnsCache = newDNSCache(e.cfg.DNSTTL, e.cfg.DNSMaxEntries)
```
instead of the hardcoded `newDNSCache(5 * time.Minute)`.

## 3. Update Default Config Generation

In `cmd/proxy-pac/main.go`, the `defaultConfigYAML` function generates the default config. Update the PAC section to include:

```yaml
pac:
  local_ip: ""
  dns_timeout: 500ms
  eval_timeout: 200ms
  enable_dns_cache: false
  dns_max_entries: 100      # new
  dns_ttl: 5m               # new
  cache_enabled: false
  cache_max_size: 10000
  cache_ttl: 10m
```

## 4. Admin UI

The admin UI's config edit page uses the `FullConfig` struct in `admin/handlers.go`. Add the new fields there:

```go
PAC struct {
    LocalIP        string        `yaml:"local_ip" json:"local_ip"`
    DNSTimeout     time.Duration `yaml:"dns_timeout" json:"dns_timeout"`
    EvalTimeout    time.Duration `yaml:"eval_timeout" json:"eval_timeout"`
    EnableDNSCache bool          `yaml:"enable_dns_cache" json:"enable_dns_cache"`
    DNSMaxEntries  int           `yaml:"dns_max_entries" json:"dns_max_entries"`  // new
    DNSTTL         time.Duration `yaml:"dns_ttl" json:"dns_ttl"`                 // new
    CacheEnabled   bool          `yaml:"cache_enabled" json:"cache_enabled"`
    CacheMaxSize   int           `yaml:"cache_max_size" json:"cache_max_size"`
    CacheTTL       time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
} `yaml:"pac"`
```

This automatically exposes them in the settings form.

# Data Flow

```
dnsResolve("example.com")
    ↓
dnsCache.get("example.com")
    - if hit: move to MRU, return
    - if miss: lookupHost() → set() (may evict LRU)
```

# Migration Notes

- Existing deployments: On restart, new DNS cache will start with default 100 entry limit. Old cached entries (in-memory) gone anyway.
- No data loss concerns (cache is ephemeral).

# Testing

- Unit test: Insert > DNSMaxEntries unique hosts; verify oldest evicted.
- Unit test: TTL expiration still works.
- Integration: Verify config round-trip includes new fields.
- Defaults: Verify default values (100, 5m) when not specified.

# Alternatives Considered

- **Use `golang-lru` library**: Rejected to avoid external dependency; existing LRU pattern in evaluator cache is clean.
- **No LRU, just TTL**: Rejected because TTL window can allow large growth before expiration.
- **Include stats**: Rejected per requirement; but could be added later if needed.
