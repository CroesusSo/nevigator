# Proposal: PAC-Enabled Proxy with TUN Support

## Problem

We need a proxy server that:
- Supports HTTP, HTTPS (CONNECT), and SOCKS5 protocols
- Uses PAC (Proxy Auto-Config) scripts to decide routing per URL/host
- Can route traffic either through a TUN device (VPN) or through a SOCKS5 upstream
- Falls back to system default route if the chosen route fails
- Provides a web UI for managing/modifying PAC rules and triggering reloads

## Solution

Build a Go-based proxy server with three core components:

1. **PAC Evaluator** - Executes JavaScript PAC files using goja, exposing standard PAC builtins (`shExpMatch`, `dnsDomainIs`, `isInNet`, `myIpAddress`, etc.). Compiles PAC to `goja.Program` for performance. Hot-reloads on file change or admin API call. Returns a single `Route` decision per evaluation. On any error, falls back to `DIRECT`.

2. **Router** - Given a `Route{Strategy, Address}` from the evaluator, establishes an outbound connection:
   - `TUN`: Dial with `LocalAddr` set to TUN interface IP (forces traffic through TUN device)
   - `SOCKS5`: Connect to upstream `host:port` and perform SOCKS5 handshake
   - `DIRECT`: `net.Dial` with no binding (uses system default route)
   If the chosen route fails (dial error, handshake failure), automatically retry with `DIRECT`.

3. **Proxy Server** - Listens for client connections on HTTP (CONNECT) and SOCKS5 ports. For each request, extracts destination host, calls `Evaluator.Evaluate(host)`, then `Router.Route(clientConn, route)`, and finally relays data bidirectionally.

4. **Admin UI & API** - Simple web interface (port 9090) to:
   - View/edit PAC file (text editor with syntax highlighting)
   - Test a URL against current PAC
   - Save changes → writes PAC file
   - Trigger manual reload via POST `/admin/pac/reload`
   - File watcher automatically reloads on disk changes

## Capabilities

This change provides the following capability:

- **pac-proxy-tun**: A proxy server with PAC evaluation, TUN/SOCKS5/DIRECT routing, and admin UI.

## Architecture

```
Client ──▶ Proxy Server (HTTP/SOCKS5)
                │
                ├─▶ PAC Evaluator (goja) ──▶ Route
                │        │
                │        └─ file watcher + API reload
                │
                └─▶ Router
                     ├─ TUN? dial(LocalAddr=tunIP)
                     ├─ SOCKS5? dial(upstream) + handshake
                     └─ DIRECT? net.Dial()
                         │
                         ▼
                     Relay client ↔ outbound
```

## Non-goals

- Implementing full PAC spec extensions beyond standard builtins
- UDP proxy support (only TCP)
- Connection pooling (per-request connection)
- Load balancing (single upstream per strategy)
- Authentication (proxy clients connect without auth)
- Clustering/distributed mode
- Custom PAC language (use standard JavaScript)

## Alternatives Considerated

- **Write PAC evaluator in pure Go** (no JS): Would break compatibility with existing PAC scripts. Rejected.
- **Support PAC chains** (e.g., "SOCKS5 a; SOCKS5 b; DIRECT"): Adds complexity. Fallback to DIRECT covers most use cases. Can revisit if needed.
- **Write raw packets to TUN device**: Would require layer 3 processing, impossible for stream proxy. Binding to TUN IP is sufficient.
- **Integrate with existing proxy (Squid, V2Ray)**: Reinventing is simpler for this specific use case; avoids dependency bloat.

## Dependencies

- Go 1.21+
- `github.com/dop251/goja` - JavaScript runtime
- `golang.org/x/net/proxy` - SOCKS5 client (optional, could implement minimal SOCKS5)
- `github.com/fsnotify/fsnotify` - file watching
- Standard library: `net`, `net/http`, `sync`, `log`

## Configuration

```yaml
proxy:
  http_listen: 0.0.0.0:8080
  socks5_listen: 0.0.0.0:1080
  admin_listen: 127.0.0.1:9090

pac:
  file: /etc/proxy.pac
  local_ip: ""  # IP for myIpAddress(); auto-detected from tun0 if empty
  dns_timeout: 500ms
  eval_timeout: 200ms
  enable_dns_cache: true
  fallback_route:
    strategy: DIRECT

tun:
  device: tun0
  # ip is auto-discovered from interface
```

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| TUN device absent or IP changes | TUN routes fail → fallback to DIRECT | Auto-detect TUN IP at startup and on reload; log warnings; monitor metrics |
| PAC script blocks (DNS, infinite loop) | Proxy stalls | Per-evaluation timeout (200ms) + `vm.Interrupt`; DNS calls with short timeout |
| SOCKS5 upstream unreachable | Fallback to DIRECT | Acceptable; log failure |
| goja memory leak | OOM over time | Use fresh VM per evaluation; no pooling; monitor |
| Admin edits bad PAC | Proxy falls back to DIRECT | Validate compile on Save; keep previous version; reject invalid syntax |

## Success Criteria

- HTTP CONNECT and SOCKS5 clients can successfully proxy
- PAC file hot-reload works without restart
- `TUN` route forces traffic through TUN device (verified via source IP)
- `DIRECT` route respects system default routing
- Admin UI allows editing, saving, and testing PAC rules
- Proxy recovers automatically when TUN/SOCKS5 becomes available after downtime
- < 5ms overhead for PAC evaluation on average
- No crashes on malformed PAC or client inputs

---

## Open Questions (to resolve during design)

1. Should we support PAC chains (multiple fallback proxies)? Could be added later.
2. Should myIpAddress() return TUN IP or system primary IP? Probably TUN IP.
3. What should be the default behavior if no PAC file exists? Fallback to DIRECT immediately.
4. Should the Admin API require authentication? Likely localhost-only sufficient; can add basic auth if exposed.
5. Metrics format: Prometheus or simple log counters?
6. Should we pre-warm goja VM pool to reduce GC pressure? Probably not needed.
