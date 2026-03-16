# Design: PAC-Enabled Proxy with TUN Support

## 1. System Overview

The system consists of four main components:

1. **Proxy Server** - Accepts HTTP CONNECT and SOCKS5 client connections
2. **PAC Evaluator** - Compiles and executes PAC scripts to decide routing
3. **Router** - Establishes outbound connections according to the chosen route
4. **Admin Service** - Web UI and API for PAC management

All components run in a single Go process.

## 2. Component Specifications

### 2.1 PAC Evaluator

**Responsibility**: Load, compile, cache, and execute PAC JavaScript to produce routing decisions.

**Configuration**:
```go
type EvaluatorConfig struct {
    PACPath       string        // Path to PAC file
    LocalIP       string        // Value returned by myIpAddress()
    DNSTimeout    time.Duration // Timeout for dnsResolve/isResolvable
    EvalTimeout   time.Duration // Max time for Evaluate()
    EnableDNSCache bool         // Enable in-memory DNS cache
    FallbackRoute Route        // Used when PAC fails
}
```

**Interface**:
```go
type Evaluator interface {
    // Evaluate returns a single Route decision for the given host.
    // If PAC evaluation fails or times out, returns FallbackRoute.
    Evaluate(host string) Route

    // Reload forces recompilation of the PAC file.
    // Uses current file on disk. Errors are logged but not propagated.
    Reload()

    // Close stops background watchers and releases resources.
    Close() error
}
```

**Implementation Details**:

- Use `github.com/dop251/goja` for JavaScript execution.
- On startup: read PAC file, create `goja.Runtime`, register builtins, compile to `goja.Program`, store in `atomic.Value`.
- Register builtins:
  - `shExpMatch(str, pattern)` → convert shell pattern to regex, match
  - `dnsDomainIs(domain, pattern)` → `strings.HasSuffix(lower(domain), lower(pattern))`
  - `isInNet(ip, pattern, mask)` → parse CIDR, check containment
  - `myIpAddress()` → return `cfg.LocalIP`
  - `dnsResolve(hostname)` → perform DNS lookup with `cfg.DNSTimeout`, cache results if enabled
  - `isResolvable(hostname)` → call `dnsResolve` and check result
- `Evaluate(host)`:
  1. Load compiled `goja.Program` from atomic.Value.
  2. Create fresh `goja.Runtime` VM (goroutine-safe).
  3. Register builtins on this VM.
  4. Run the program to define `FindProxyForURL`.
  5. Call `FindProxyForURL("", host)` (URL empty string per SOCKS/HTTP CONNECT typical usage).
  6. Capture return value, parse with `parsePACResult(...)`.
  7. If any error (compilation, missing function, runtime panic, timeout), return `FallbackRoute`.
  8. Enforce `EvalTimeout` using `select` on result channel + `vm.Interrupt()`.
- Hot reload:
  - Start `fsnotify.Watcher` on PAC file's directory.
  - On `Write` event for PAC file, call `Reload()` in a separate goroutine.
  - Also expose `/admin/pac/reload` endpoint that calls `Reload()`.
  - During `Reload()`: read file, compile, validate `FindProxyForURL` exists; if successful, atomically swap compiled program; if fails, log error and keep old version.
- DNS cache (optional):
  - Simple `sync.Map` mapping host → `{addrs []string, expires time.Time}`.
  - TTL 5 minutes. Cache only A/AAAA lookups.

**Error Handling**:
- All PAC errors (syntax, runtime panic, timeout) are logged and result in `FallbackRoute`.
- Never propagate PAC errors to client; maintain connectivity via fallback.

### 2.2 Router

**Responsibility**: Convert a `Route` into an established outbound connection, with fallback to `DIRECT` on failure.

**Configuration**:
```go
type RouterConfig struct {
    TUNIP net.IP // IP address of TUN interface; if nil, TUN routes fail immediately
}
```

**Interface**:
```go
type Router interface {
    // Route establishes a connection to the destination implied by clientConn.RemoteAddr(),
    // following the specified route. If route.Strategy is not DIRECT and establishment fails,
    // automatically retry with Route{Strategy: "DIRECT"}.
    // Returns the established outbound connection, or error if even DIRECT fails.
    Route(clientConn net.Conn, route Route) (net.Conn, error)

    Close() error
}
```

**Implementation Details**:

- `Route(clientConn, route)`:
  1. Extract destination string: `clientConn.RemoteAddr().String()` (format `host:port`).
  2. Attempt dial according to `route.Strategy`:
     - `TUN`: `dialer := &net.Dialer{LocalAddr: &net.TCPAddr{IP: cfg.TUNIP}}; conn, err := dialer.Dial("tcp", dest)`
     - `SOCKS5`: `conn, err := proxy.SOCKS5("tcp", route.Address, nil, proxy.Direct).Dial("tcp", dest)` using `golang.org/x/net/proxy`
     - `DIRECT`: `conn, err := net.Dial("tcp", dest)`
  3. If `err != nil` and `route.Strategy != "DIRECT"`:
     - Log failure (including route details).
     - Retry with `Route{Strategy: "DIRECT"}` (once).
  4. Return connection or error.
- Timeouts: Use default `net.Dialer` timeout (typically no timeout for TCP connect; could add config). Consider setting a configurable `DialTimeout` (e.g., 30s) to avoid long hangs.

**Note**: The outbound connection is independent of the client connection. The caller is responsible for relaying data between `clientConn` and the returned `outboundConn`.

### 2.3 Proxy Server

**Responsibility**: Accept client connections, extract destination, consult PAC evaluator, invoke router, and relay data.

**Protocol Handlers**:

1. **HTTP CONNECT** (port 8080):
   - Accept HTTP request, parse `CONNECT host:port HTTP/1.1`.
   - Extract `host:port`.
   - Call `evaluator.Evaluate(host)` to get `route`.
   - Call `router.Route(clientConn, route)` to get `outboundConn`.
   - Respond with `200 Connection Established` (or error if no route succeeded).
   - Start bidirectional copy: `go copy(outbound, client)` and `go copy(client, outbound)`.

2. **SOCKS5** (port 1080):
   - Implement minimal SOCKS5 handshake (version 5, no auth or username/password).
   - Read client's CONNECT request: `destAddr` (domain or IP, port).
   - Extract host (resolve domain if needed).
   - Call `evaluator.Evaluate(host)`.
   - Call `router.Route(clientConn, route)`.
   - Send SOCKS5 reply to client (success/failure).
   - Relay if successful.

**Configuration**:
```go
type ProxyConfig struct {
    HTTPListen    string
    SOCKS5Listen  string
    AdminListen   string
    PACPath       string
    TUNDevice     string
    TUNIP         net.IP // auto-discovered if empty
}
```

**Initialization**:
1. Discover TUN IP from interface (if configured):
   ```go
   iface, _ := net.InterfaceByName(cfg.TUNDevice)
   for _, addr := range iface.Addrs {
       if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
           tunIP = ipnet.IP
           break
       }
   }
   ```
2. Create `Evaluator` with `LocalIP = tunIP.String()` (or override).
3. Create `Router` with `TUNIP`.
4. Start HTTP server on `cfg.HTTPListen` with handlers for `/` (CONNECT) and `/admin/*`.
5. Start SOCKS5 listener (custom implementation or use `golang.org/x/net/proxy` server? Could write simple one).
6. Start fsnotify watcher inside evaluator.

**Graceful Shutdown**:
- Capture `os.Interrupt` and `syscall.SIGTERM`.
- Close all listeners.
- Wait for active connections to finish (with timeout) or just close them.

### 2.4 Admin Service

**Responsibility**: Provide UI and API for PAC management.

**HTTP Endpoints** (all bound to `AdminListen`, typically localhost):
- `GET /admin` → serve HTML UI (static file embedded or templates)
- `GET /admin/pac` → returns raw PAC file content (text/javascript)
- `POST /admin/pac` → expects `text/plain` body; writes to `PACPath`; triggers `Evaluator.Reload()`
- `POST /admin/pac/reload` → triggers `Evaluator.Reload()` without write
- `GET /admin/test?url=...` → evaluates URL against current PAC, returns JSON `{ "route": { "strategy": "...", "address": "..." } }`
- `GET /admin/status` → returns JSON `{ "tun_ip": "...", "pac_loaded": true, "uptime": ... }`

**UI Features**:
- Textarea with PAC content (prefill from GET `/admin/pac`).
- Syntax highlighting (optional, use CodeMirror or simple `<textarea>`).
- Save button → POST `/admin/pac`.
- Reload button → POST `/admin/pac/reload`.
- Test section: input URL → click Test → show chosen route.
- Status display: TUN IP, PAC last loaded time, fallback route.

## 3. Data Types

```go
type Route struct {
    Strategy string // "TUN", "SOCKS5", "DIRECT"
    Address  string // required for SOCKS5, ignored otherwise
}

func parsePACResult(s string) Route {
    // Parse PAC return string (first token only):
    // "DIRECT" → {Strategy: "DIRECT"}
    // "SOCKS5 1.2.3.4:1080" → {Strategy: "SOCKS5", Address: "1.2.3.4:1080"}
    // "PROXY ..." → could map to HTTP upstream (out of scope for MVP), treat as SOCKS5? Not standard. We only support SOCKS5 upstream. So ignore PROXY.
    // We only support TUN and SOCKS5 as explicit strategies; anything else → DIRECT.
    tokens := strings.Fields(s)
    if len(tokens) == 0 {
        return Route{Strategy: "DIRECT"}
    }
    switch strings.ToUpper(tokens[0]) {
    case "TUN":
        return Route{Strategy: "TUN"}
    case "SOCKS5":
        if len(tokens) >= 2 {
            return Route{Strategy: "SOCKS5", Address: tokens[1]}
        }
        return Route{Strategy: "DIRECT"}
    default:
        return Route{Strategy: "DIRECT"}
    }
}
```

Note: PAC spec includes `PROXY` for HTTP proxies, but our router doesn't support upstream HTTP proxies (only TUN, SOCKS5, DIRECT). If PAC returns `PROXY`, treat as `DIRECT` or perhaps implement later. For MVP, map to `DIRECT`.

## 4. Sequence Diagrams

### 4.1 Client Request Flow

```
Client          Proxy          Evaluator       Router         Outbound
  |--CONNECT---->|               |               |               |
  |              |--Evaluate(host)------------>|               |
  |              |<--Route(TUN)-|               |               |
  |              |--Route(TUN)----------------->|               |
  |              |              |               |--dial(Local=TUN_IP)
  |              |              |               |<--conn--------|
  |              |<--200 OK-----|               |               |
  |<--200 OK-----|               |               |               |
  |===== relay ==============================>|               |
  |              |<== bidirectional copy ====>|               |
```

If TUN fails:
```
  |              |--Route(TUN)----------------->|   (fails)
  |              |<--Route(DIRECT)-------------|   (fallback)
  |              |--Route(DIRECT)------------->|
  |              |--dial()-------------------->|
  |              |<--conn---------------------|
  |              |<--200 OK (after success)-|
```

### 4.2 PAC Reload Flow

```
Admin UI       Proxy (API)    Evaluator       FS watcher
  |--Save PAC-->|              |               |
  |             |--write file->|               |
  |             |              |<--Write event-|
  |             |--Reload()--->|               |
  |             |<-200 OK------|               |
  |<--200 OK----|              |               |
```

## 5. Error Handling & Fallback

- **PAC compile error**: Keep old compiled version if exists; else use `FallbackRoute` (DIRECT). Log error.
- **PAC runtime error** (panic, missing function): Use `FallbackRoute`. Log error.
- **PAC timeout**: `vm.Interrupt()`, use `FallbackRoute`. Log warning.
- **Router: TUN or SOCKS5 dial failure**: Retry with `DIRECT` once. Log failure.
- **Router: DIRECT failure**: Return error to client → connection closed. Log as client error.
- **TUN IP not configured**: TUN routes fail immediately → fallback to DIRECT. Log warning at startup.

All fallbacks happen **transparently** to the client. The client sees either a successful connection or a final error if all (including DIRECT) fail.

## 6. Performance Considerations

- **PAC compilation** happens once per change; negligible.
- **PAC evaluation**: Creating fresh `goja.Runtime` per request has overhead (~100-200 µs). Acceptable for most uses. Could later implement VM pool if needed.
- **DNS caching**: Reduces latency for PAC scripts using `dnsResolve`. Cache hit ~µs.
- **Router**: Simple dial; overhead minimal.
- **Relay**: `io.Copy` uses large buffers; efficient.

## 7. Testing Strategy

**Unit Tests**:
- `parsePACResult` with various inputs.
- Each builtin function (`shExpMatch`, `dnsDomainIs`, etc.) with edge cases.
- `Evaluator.Evaluate` with simple PAC scripts (mock `net.LookupHost` if needed).
- `Router.Route` for each strategy, mocking network calls.

**Integration Tests**:
- End-to-end: HTTP CONNECT to proxy with PAC that returns TUN/SOCKS5/DIRECT; verify traffic flows correctly.
- Simulate TUN down: verify fallback to DIRECT.
- PAC reload: modify file, ensure new rules take effect.
- Slow PAC script with DNS: ensure timeout and fallback.

**Manual Tests**:
- Use browser with `http://proxy:8080` as HTTP proxy and PAC URL `http://proxy:9090/admin/pac`.
- Test with real PAC scripts (e.g., fromcorporate environments).

## 8. Monitoring & Metrics

Expose Prometheus metrics (or simple `/metrics` endpoint):

- `pac_eval_duration_seconds` (histogram)
- `pac_eval_total` (counter by outcome: success, error, timeout)
- `route_attempt_total` (counter by strategy)
- `route_fallback_total` (counter from TUN/SOCKS5 to DIRECT)
- `connections_total` (by client, protocol)
- `active_connections` (gauge)

## 9. Security Considerations

- Admin API binds to localhost by default. If exposed externally, add basic auth.
- PAC file validated on load; if syntax error, old version retained.
- goja VM isolated per request; no persistent state.
- No arbitrary file access from PAC script (builtins only).
- No network access from PAC script except `dnsResolve` (controlled with timeout).
- Limit memory per evaluation indirectly via per-request VM (GC'd after function returns).
- If needed, could add `vm.Interrupt` after instruction count, but timeout simpler.

## 10. Deployment

### 10.1 Systemd Service

```
[Unit]
Description=PAC-enabled proxy with TUN support
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=proxy
ExecStart=/usr/local/bin/proxy-pac --config /etc/proxy-pac.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

### 10.2 Docker

Requires `--cap-add=NET_ADMIN` to write to TUN (if the proxy itself creates/manages TUN; but our design assumes TUN is created by external VPN). The proxy only needs to read TUN interface IP. No special capabilities needed if TUN exists.

Mount `/etc/proxy-pac.yaml` and `/etc/proxy.pac`.

### 10.3 Configuration Management

Admin UI writes to `PACPath`. Ensure file permissions restrict to admin user only.

## 11. Open Decisions

1. **SOCKS5 upstream auth**: Should we support username/password? Could add optional `username:password@host:port` parsing. defer.
2. **HTTP upstream proxy**: Support `PROXY host:port` from PAC? Could be added later.
3. **DNS cache TTL**: 5 minutes arbitrary; tweakable?
4. **TUN IP refresh**: Should router periodically re-read TUN IP? Or just on reload? Probably on reload sufficient.
5. **Metrics backend**: Prometheus vs log-based? Start with Prometheus endpoint.

## 12. Future Enhancements

- Chain multiple routes (not just single + fallback).
- Visual PAC rule editor (not just textarea).
- PAC script import/export.
- Connection pooling for SOCKS5 upstream.
- UDP support (for DNS forwarding?).
- Health checks (probe TUN, SOCKS5 upstream).
- Rate limiting per client.
- Client authentication (basic, IP whitelist).
