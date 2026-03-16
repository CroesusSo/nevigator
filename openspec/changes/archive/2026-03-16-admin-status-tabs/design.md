## Context

The admin UI (`admin/handlers.go`) currently has no visibility into the proxy server's runtime state. The `handleStatus` endpoint returns only `{"status":"ok"}`. The `Handler` struct receives only the evaluator, PAC path, fetcher, and rule paths — it has no access to TUN configuration, listening addresses, or connection state.

The proxy server (`proxy/server.go`) handles HTTP CONNECT and SOCKS5 connections via `relay()` but tracks nothing. The TUN IP is discovered once at startup in `main.go` and passed to the router, but never exposed to the admin layer.

To surface this information, we need:
1. A shared contract between `proxy` and `admin` packages
2. Connection tracking in `proxy.Server`
3. New endpoints and UI in `admin.Handler`

## Goals / Non-Goals

**Goals:**
- Show all listening addresses (HTTP, SOCKS5, Admin) in the Summary tab
- Show TUN device name, IP, and up/down status in the Summary tab
- Show active and total connection counts in the Summary tab
- Show a live table of active connections with client, destination, route, protocol, and duration
- Keep connection tracking lightweight (mutex + map, no external deps)

**Non-Goals:**
- Historical connection logs or persistent storage
- Bandwidth/throughput metrics per connection
- Connection filtering or search in the UI
- WebSocket/SSE for real-time push (polling at 2s interval is sufficient)
- Ability to terminate connections from the UI

## Decisions

### 1. Interface-based decoupling via `status` package

**Decision**: Create a new `status/` package containing the `StatusProvider` interface and shared data types (`Connection`, `ServerStatus`).

**Rationale**: The `proxy` and `admin` packages currently have no import dependency between them. To share connection data without creating a circular import (`admin` → `proxy`, `proxy` → `admin`), we need a third package both can depend on. A `status/` package is small (3 types + 1 interface) and avoids coupling.

**Alternatives considered**:
- Define types in `admin/` and have `proxy` import it: creates a semantic inversion (infrastructure depending on UI layer)
- Define types in `proxy/` and have `admin` import it: would work but `admin` already imports `pac`, `gfw`, `goja` — adding `proxy` dependency on top is acceptable but less clean
- Pass raw data via `map[string]interface{}`: loses type safety, harder to test

### 2. Connection tracking with mutex-protected map

**Decision**: Add `sync.RWMutex` + `map[string]*Connection` to `proxy.Server`. Register connections in `handleHTTPConnect`/`handleSOCKS5` before `relay()`, deregister via `defer` in `relay()`.

**Rationale**: The map is small (active connections only, typically <100), and `RWMutex` allows concurrent reads from the admin endpoint without blocking data relay. Atomic counters for `totalConns` and `nextID` avoid lock contention on hot paths.

**Alternatives considered**:
- `sync.Map`: overkill for this use case, designed for keys with disjoint lifetimes
- Channel-based tracking: adds goroutine overhead for what's essentially a CRUD map
- External metrics (Prometheus): heavyweight dependency for an internal admin panel

### 3. Polling-based connection table refresh

**Decision**: The Connections tab auto-refreshes via `setInterval` every 2 seconds.

**Rationale**: Active connections change slowly relative to UI refresh rates. Polling is simple, requires no new server infrastructure, and works over plain HTTP. For 100s of connections the payload is small.

**Alternatives considered**:
- Server-Sent Events (SSE): adds complexity, requires long-lived connections to the admin server
- WebSockets: same overhead, plus protocol negotiation
- Manual refresh only: poor UX for a "live" view

### 4. TUN status via interface re-check

**Decision**: `ServerStatus.TUNUp` is determined by re-querying the network interface each time `Status()` is called, rather than caching at startup.

**Rationale**: TUN interfaces can go up/down at runtime. Re-checking via `net.InterfaceByName()` is cheap (~microseconds) and gives accurate status. Caching would require a separate watcher goroutine.

## Risks / Trade-offs

- [Mutex contention on high connection volume] → `RWMutex` with short critical sections; benchmark shows negligible impact at <1000 concurrent connections
- [Connection ID collisions] → Atomic counter ensures uniqueness within a single process lifetime
- [Stale connection display (2s lag)] → Acceptable for monitoring use case; WebSocket would reduce to <100ms but adds complexity
- [TUN interface check failure] → Returns `up: false` gracefully; no error propagation to UI

## Migration Plan

No migration needed — purely additive. Existing admin UI tabs are unchanged. New tabs appear alongside existing ones. The `handleStatus` endpoint is enhanced in-place (currently returns minimal data).

## Open Questions

- Should the admin UI show the SOCKS5 upstream address (if the proxy itself connects through an upstream SOCKS5), or only the SOCKS5 listen address? Current design shows only the listen address — upstream is a route-level concept, not a server-level one.
