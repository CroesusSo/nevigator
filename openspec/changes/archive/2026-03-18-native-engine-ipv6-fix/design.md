# Native Engine IPv6 Fix – Design

## Architecture Context

The native rule engine (`pac/native_engine.go`) provides high-performance PAC evaluation by compiling rules into in-memory data structures:

- **Exact map** (`map[string]*ruleMeta`) for O(1) host lookups
- **Reversed-label domain suffix trie** for efficient `.example.com` matching
- **Host globs** (compiled regexes) for shell pattern matching like `*.onion`

```
Evaluate(host):
  1. Clean host (lowercase, strip port)
  2. Lookup exactMap[host]
  3. Query suffixTrie.match(host)
  4. Scan hostGlobs for matches, pick lowest index
  5. Fallback if no match
```

### Current IPv6 Breakage

The cleaning step uses:
```go
if idx := strings.IndexByte(host, ':'); idx >= 0 {
    host = host[:idx]  // WRONG: truncates IPv6 at first colon
}
```

For IPv6 address `2001:db8::1`:
- `idx = 4` (position of first colon)
- `host[:4] = "2001"` → no rule matches

The same bug exists in:
- `Build()` for exact rules (`case gfw.MatchExact`)
- `domainSuffixTrie.match()`

Additionally, hostGlobs loop:
```go
for _, hg := range e.hostGlobs {
    if hg.regex.MatchString(host) {
        if hg.meta.index < bestIndex { ... }
        break  // ← stops at first match, ignoring rule ordering
    }
}
```

## Proposed Fixes

### 1. Use `net.SplitHostPort` for port stripping

Replace all three colon-truncation blocks with:

```go
if h, _, err := net.SplitHostPort(host); err == nil {
    host = h
}
```

This correctly handles:
- IPv6 with port: `"[2001:db8::1]:443"` → `"2001:db8::1"`
- IPv6 without port: `"2001:db8::1"` → unchanged (no error)
- IPv4 with port: `"example.com:8080"` → `"example.com"`
- Plain hostnames: no change

### 2. Remove `break` in hostGlobs loop

Change:
```go
if hg.regex.MatchString(host) {
    if hg.meta.index < bestIndex {
        bestIndex = hg.meta.index
        bestRoute = hg.meta.route
    }
    break
}
```
To:
```go
if hg.regex.MatchString(host) {
    if hg.meta.index < bestIndex {
        bestIndex = hg.meta.index
        bestRoute = hg.meta.route
    }
    // no break – continue to find lowest index among all matches
}
```

This respects rule ordering: if both `*.example.com` (index 0) and `*` (index 1) match, the former wins.

### 3. Add net import

`native_engine.go` currently imports:
```go
import (
    "encoding/json"
    "fmt"
    "log"
    "regexp"
    "strings"
    "github.com/yourorg/proxy-pac/gfw"
)
```
Add `"net"`.

## Test Plan

Unit tests in `pac/native_engine_test.go`:

1. **TestExact_IPv6**: Rule with IPv6 literal matches evaluation with same IPv6.
2. **TestHostGlob_IPv6**: Pattern like `2001:db8::*` matches appropriate hosts.
3. **TestSuffix_IPv6_NoMatch**: Domain suffix patterns do not match IPv6 (ensures we didn't break suffix logic).
4. **TestHostGlob_Priority**: Two overlapping globs, verify lower index wins.

Existing tests must continue to pass (backward compatibility).

## Data Flow After Fix

```
Input host: "2001:db8::1"
→ Evaluate: net.SplitHostPort → "2001:db8::1"
→ exactMap["2001:db8::1"] lookup (if present)
→ suffixTrie.match("2001:db8::1") → nil (IPv6 not a domain)
→ hostGlobs: regex.MatchString("2001:db8::1") matches pattern "2001:db8::*"
→ Returns matching route with lowest index
```

## Edge Cases

- **Bracketed IPv6**: `"[::1]:443"` already split by caller → `"::1"`. SplitHostPort handles it anyway.
- **IPv6 with zone ID**: `"fe80::1%eth0"` – valid but rare; SplitHostPort may fail; host remains unchanged (acceptable).
- **Malformed inputs**: SplitHostPort returns error → host unchanged, matching proceeds.

## Migration Impact

No PAC format changes. Existing rule editor outputs unaffected. Only internal engine behavior improves.
