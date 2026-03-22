# Capability: PAC-Enabled Proxy with TUN Routing

## Overview

This system implements a high-performance proxy server that:
- Accepts HTTP CONNECT and SOCKS5 client connections
- Evaluates **structured routing rules** (not JavaScript) to determine outbound path
- Supports three routing strategies:
  - `TUN`: force traffic through a TUN device (VPN tunnel)
  - `SOCKS5 upstream`: route through a remote SOCKS5 server
  - `DIRECT`: use system default routing (respects OS routing table)
- Falls back to `DIRECT` if the chosen route fails
- Provides a web-based admin UI for rule management and hot-reload

**Important:** The proxy uses a **native Go rule engine only**. It does **not** execute arbitrary JavaScript PAC files. Routing decisions are made from structured JSON rules embedded in the PAC file via the `__PAC_RULES__` variable.

## Key Interfaces

### Route

```go
type Route struct {
    Strategy   string // "TUN", "SOCKS5", "DIRECT", "EXTERNAL", "DEFAULT"
    Address    string // required for SOCKS5
    ExternalID string // used when Strategy == "EXTERNAL"
}
```

### Evaluator

```go
type Evaluator interface {
    Evaluate(host string) Route
    Reload()
    Close() error
}
```

The evaluator loads a PAC file containing `__PAC_RULES__` JSON, builds indexes (exact map, suffix trie, host globs), and returns the best matching route for a given hostname.

### Router

```go
type Router interface {
    Route(dest string, route Route) (net.Conn, error)
    Close() error
}
```

### Proxy Server

- HTTP CONNECT: `:8080`
- SOCKS5: `:1080`
- HTTPS CONNECT (TLS-wrapped): `:8443` (optional)
- Admin UI/API: `:9090`

## Behavior

1. Client connects and requests a destination `host:port` (via CONNECT or SOCKS5 CONNECT)
2. Proxy parses destination hostname from request
3. Calls `Evaluator.Evaluate(host)` to obtain a `Route`
4. Router establishes outbound connection according to `route`:
   - `TUN`: binds socket to TUN interface IP
   - `SOCKS5`: connects to upstream SOCKS5 server and performs handshake
   - `EXTERNAL <id>`: looks up proxy from registry by ID
   - `DEFAULT`: uses the configured default external proxy
   - `DIRECT`: normal `net.Dial` (OS routing)
5. If route establishment fails (and `route != DIRECT`), Router automatically retries with `DIRECT`
6. On success, proxy relays bytes bidirectionally between client and outbound
7. Admin modifies PAC file → evaluator reloads → new routing decisions take effect

## PAC File Format

PAC files **must** contain a JavaScript variable `__PAC_RULES__` with a JSON object:

```javascript
var __PAC_RULES__ = {"version":1,"rules":[
  {"id":"1","match":"exact","value":"example.com","route":"TUN"},
  {"id":"2","match":"domain_suffix","value":".google.com","route":"SOCKS5 127.0.0.1:9050"},
  {"id":"3","match":"host_glob","value":"*.onion","route":"DIRECT"}
]};
function FindProxyForURL(url, host) { return 'DIRECT'; }
```

The `FindProxyForURL` function is **ignored**; it exists only to satisfy legacy PAC file parsers. The actual routing logic is entirely driven by `__PAC_RULES__`.

### Rule Fields

- `id`: unique string identifier
- `match`: match type — `exact`, `domain_suffix`, `host_glob` (URL-glob not supported)
- `value`: pattern to match (exact hostname, leading dot for suffix, `*` wildcards for host glob)
- `route`: routing decision — `"TUN"`, `"SOCKS5 host:port"`, `"EXTERNAL <id>"`, `"DEFAULT"`, `"DIRECT"`
 - `enabled`: boolean (default true)

Rules are evaluated in ascending `id` order. The native engine always uses deepest match semantics:
- For `domain_suffix`: the longest matching suffix wins (most specific)
- For `host_glob`: patterns are internally sorted by specificity (longer, fewer wildcards first), with `id` as tie-breaker

### Example PAC Generation

The `gfw.GeneratePAC` function produces `__PAC_RULES__` from user and GFW rules. This is used by the admin UI and GFW fetcher.

## Configuration

```yaml
proxy:
  http_listen: 0.0.0.0:8080
  socks5_listen: 0.0.0.0:1080
  admin_listen: 127.0.0.1:9090
  pac_path: /etc/proxy.pac
  tun_device: tun0
  default_external_proxy: ""
  external_proxies: []  # list of {id, address, username, password}

 pac:
   enable_dns_cache: true  # DNS cache for internal lookups (not used for user PAC)
   dns_max_entries: 100    # max DNS cache entries (0 = unlimited)
   dns_ttl: 5m             # DNS cache TTL
   cache_enabled: true     # enable PAC result caching
   cache_max_size: 10000   # max cached hosts
   cache_ttl: 10m          # cache TTL
 ```

## Native Rule Engine

The native engine provides **>100× performance improvement** over a JavaScript interpreter, with evaluation typically in sub‑microseconds.

### Indexes

- **Exact map**: `map[host]route` for O(1) exact matches
- **Domain suffix trie**: reversed‑label trie for efficient suffix matching (O(depth))
- **Host glob patterns**: compiled regexes stored in a slice; evaluated in order

### Performance

- Un cached evaluation: < 1 µs for simple rule sets
- LRU cache (optional) can serve repeated hosts in < 100 ns
- Memory footprint: ~1 MB for 10k cached entries

## Non-Functional

- **Performance**: Sub-microsecond native evaluation; cache hit < 100 ns
- **Reliability**: Hot‑reload without dropping connections; automatic fallback to DIRECT
- **Safety**: Syntax errors in PAC file are logged; fallback route used; connectivity never broken
- **Resource**: Baseline memory ~5 MB; cache adds ~1 MB for 10k entries

## Dependencies

- Go 1.21+
- `github.com/fsnotify/fsnotify` (file watching)
- `gopkg.in/yaml.v3` (config parsing)
- `github.com/yourorg/proxy-pac/gfw` (rule generation)

*Note:* The `github.com/dop251/goja` JavaScript runtime is **not** used.

## Testing

Unit tests for native engine (exact, suffix, host glob, deepest match), cache, DNS cache; integration tests for HTTP CONNECT, SOCKS5, HTTPS proxy; end‑to‑end with generated PAC.

## Future Extensions

- Upstream HTTP PROXY support
- Multiple route chains
- Connection pooling
- Client authentication
- Prometheus metrics
- PAC visual editor
- Eventually make `deepest_match` the default (with deprecation period)

## ADDED Requirements (from external-proxy-registry)

The full external proxy registry and DEFAULT behavior are documented in `openspec/specs/external-proxy-registry/spec.md`. Key points:

- Registry of external SOCKS5 proxies with optional credentials
- Manageable via Admin UI and `/api/proxies` REST API
- `EXTERNAL <id>` and `DEFAULT` route strategies look up proxies from this registry
- Validation to prevent loops and reserved IDs
- Atomic config updates
