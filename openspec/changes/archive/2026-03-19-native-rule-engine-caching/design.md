# Native Rule Engine and Caching – Design

## Architecture
The `Evaluator` will support two modes:
- **JS mode**: compiles PAC to `goja.Program` (existing).
- **Native mode**: builds `*NativeRuleEngine` from `__PAC_RULES__`.

Mode selection in `reload()`:
```go
if strings.Contains(pacContent, "var __PAC_RULES__") {
    engine, err := NewNativeRuleEngine(pacContent, cfg.FallbackRoute)
    if err != nil {
        log.Printf("[ERROR] Native engine build failed: %v; falling back to JS", err)
        // fallback to JS compilation
    } else {
        e.current.Store(engine)
        e.engineType = "native"
    }
} else {
    // existing JS compile path
}
```

## Native Rule Engine Data Structures

### RuleMeta
```go
type ruleMeta struct {
    index int          // position in PAC (0 = first)
    route pac.Route    // resolved route (Strategy, Address, ExternalID)
}
```

### Exact Map
```go
exactMap map[string]*ruleMeta  // lowercased host → earliest rule
```
- Insertion: keep rule with smallest index for each host.
- Lookup: O(1).

### Suffix Trie
Label‑based reversed trie:
```go
type trieNode struct {
    children map[string]*trieNode
    rule     *ruleMeta  // rule that terminates at this suffix (earliest index)
}
type DomainSuffixTrie struct{ root *trieNode }
```
- Insert: split pattern into labels (reverse order), create nodes, store rule if earlier.
- Match(host):
  1. Lowercase host, strip port.
  2. Split into reversed labels.
  3. Walk trie; at each node check its rule; keep the rule with smallest index among visited nodes (longest‑match‑plus‑earliest).
  Complexity O(depth) where depth = number of labels (typically 2‑5).

### Host Globs
```go
type ruleMetaRegex struct {
    meta  ruleMeta
    regex *regexp.Regexp  // compiled from shell pattern
}
hostGlobs []ruleMetaRegex  // ordered by index (earliest first)
```
- Build: deduplicate by pattern (keep earliest index), compile regex once.
- Match: iterate in order; first regex that matches host returns candidate. Early exit ensures at most a few checks.

### URL‑Glob Rules
Skipped entirely during engine construction. If the PAC contains any, they are ignored (same behavior as current JS mode where `url` argument is empty). A warning is logged.

### Engine Evaluation

```go
func (e *NativeRuleEngine) Evaluate(host string) pac.Route {
    host = normalize(host) // lowercase, strip port

    bestIndex := math.MaxInt
    var bestRoute pac.Route

    // 1. Exact candidate
    if m, ok := e.exactMap[host]; ok && m.index < bestIndex {
        bestIndex = m.index; bestRoute = m.route
    }

    // 2. Suffix candidate
    if m := e.suffixTrie.Match(host); m != nil && m.index < bestIndex {
        bestIndex = m.index; bestRoute = m.route
    }

    // 3. Host glob candidates (ordered by index)
    for _, hg := range e.hostGlobs {
        if hg.regex.MatchString(host) {
            if hg.meta.index < bestIndex {
                bestIndex = hg.meta.index; bestRoute = hg.meta.route
            }
            break // first matching glob is earliest due to ordering
        }
    }

    if bestIndex == math.MaxInt {
        return e.fallback
    }
    return bestRoute
}
```

## LRU Cache

### Structure
- `entries map[string]*cacheEntry`
- `lruList *list.List` (host strings, most recent at front)
- `maxSize int`, `ttl time.Duration`
- Stats: `hits`, `misses` (atomic)

### Operations
- `Get(host)`: lock → check expiry → if valid, move to front → return route.
- `Set(host, route)`: lock → update existing or insert → evict LRU if oversized.
- `InvalidateAll()`: clear map and list (called on PAC reload).
- Background `cleanupLoop()`: every minute removes expired entries.

### Cache Key
Only `host` string. Because URL‑glob rules are not considered, every host deterministically maps to a single route (given a fixed PAC).

## Integration with Evaluator

```go
type Evaluator struct {
    cfg       EvaluatorConfig
    current   atomic.Value // *compiledPAC or *NativeRuleEngine
    engineType string      // "js" or "native"
    cache     *EvaluatorCache
    // ...
}

func (e *Evaluator) Evaluate(host string) pac.Route {
    // 1. Try cache
    if r, ok := e.cache.Get(host); ok {
        return r
    }
    // 2. Evaluate via current engine
    var r pac.Route
    switch v := e.current.Load().(type) {
    case *NativeRuleEngine:
        r = v.Evaluate(host)
    case *compiledPAC:
        r = e.evaluateJS(v, host)
    default:
        r = e.cfg.FallbackRoute
    }
    // 3. Cache result
    e.cache.Set(host, r)
    return r
}
```

On `reload()`:
- Build new engine (native if applicable, else JS).
- `e.current.Store(engine)`
- `e.cache.InvalidateAll()`

## Error Handling
If `NewNativeRuleEngine` fails (malformed JSON, unknown match type), the error is logged and the reload falls back to JS compilation. This ensures the proxy remains operational.

## Observability
Add admin endpoint `/api/cache` to return:
```json
{
  "hits": 12345,
  "misses": 678,
  "size": 5000,
  "hit_rate": 0.948,
  "max_size": 10000,
  "ttl_seconds": 600
}
```

## Performance Expectations
- Uncached native evaluation: ~0.5–2 µs (vs 1000 µs for JS).
- Cached hit: ~0.05 µs.
- Memory overhead: trie ≈ 1-2 MB, cache ≈ 10k × (host+route) ≈ a few MB.
- CPU: negligible per request.

## Testing Strategy
- Unit tests for each index component (exactMap, suffixTrie, hostGlobs) with duplicates, ordering, case insensitivity.
- Integration tests: feed a real 4100‑rule PAC, verify routes for known hosts (e.g., "www.google.com" → DEFAULT).
- Benchmarks: compare latency and throughput of JS vs native with and without cache.
- Property‑based testing: generate random rule sets, compare native engine output with reference JS evaluator.
