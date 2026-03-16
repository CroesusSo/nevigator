# Capability: PAC-Enabled Proxy with TUN Routing

## Overview

This system implements a high-performance proxy server that:
- Accepts HTTP CONNECT and SOCKS5 client connections
- Executes PAC (Proxy Auto-Config) scripts to determine routing per destination
- Supports three routing strategies:
  - `TUN`: force traffic through a TUN device (VPN tunnel)
  - `SOCKS5 upstream`: route through a remote SOCKS5 server
  - `DIRECT`: use system default routing (respects OS routing table)
- Falls back to `DIRECT` if the chosen route fails
- Provides a web-based admin UI for PAC management and hot-reload

## Key Interfaces

### PAC Evaluator

```go
type Route struct {
    Strategy string // "TUN", "SOCKS5", "DIRECT"
    Address  string // required for SOCKS5
}

type Evaluator interface {
    Evaluate(host string) Route
    Reload()
    Close() error
}
```

### Router

```go
type Router interface {
    Route(clientConn net.Conn, route Route) (net.Conn, error)
    Close() error
}
```

### Proxy Server

- HTTP CONNECT: `:8080`
- SOCKS5: `:1080`
- Admin UI/API: `:9090`

## Behavior

1. Client connects and requests a destination host:port (via CONNECT or SOCKS5 CONNECT)
2. Proxy calls `Evaluator.Evaluate(host)` to get a single `Route`
3. Router attempts to establish outbound connection according to `route`
4. If that fails and `route != DIRECT`, Router silently retries with `DIRECT`
5. On success, proxy relays bytes between client and outbound
6. Admin modifies PAC file → UI triggers reload → Evaluator recompiles → new routing decisions

## PAC Script Requirements

- Must define `FindProxyForURL(url, host)` function
- Should return one of: `"TUN"`, `"SOCKS5 host:port"`, `"DIRECT"`
- Other return values are treated as `DIRECT`
- `myIpAddress()` returns the configured TUN IP (or static config)

## Supported Built-ins

`shExpMatch`, `dnsDomainIs`, `isInNet`, `myIpAddress`, `dnsResolve`, `isResolvable`

## Configuration

```yaml
proxy:
  http_listen: 0.0.0.0:8080
  socks5_listen: 0.0.0.0:1080
  admin_listen: 127.0.0.1:9090
  tun_device: tun0

pac:
  file: /etc/proxy.pac
  local_ip: ""            # auto-detected from tun0 if empty
  dns_timeout: 500ms
  eval_timeout: 200ms
  enable_dns_cache: true
```

## Non-Functional

- **Performance**: Sub-millisecond PAC evaluation for simple scripts; < 5ms average
- **Reliability**: Hot-reload without dropping connections; automatic fallback
- **Safety**: PAC errors never break connectivity; fallback to DIRECT
- **Resource**: ~5MB memory baseline; per-request VM ~100KB (short-lived)

## Dependencies

- Go 1.21+
- `github.com/dop251/goja` (JS runtime)
- `golang.org/x/net/proxy` (SOCKS5 client)
- `github.com/fsnotify/fsnotify` (file watching)

## Deployment

Runs as a single binary. Systemd service recommended. Requires TUN device to be present (e.g., managed by separate VPN client). Admin UI binds to localhost by default.

## Testing

Unit tests for builtins, evaluator, router; integration tests for HTTP CONNECT and SOCKS5 flows; end-to-end with real PAC scripts.

## Future Extensions

- Upstream HTTP PROXY support
- Multiple route chains
- Connection pooling
- Client authentication
- Prometheus metrics
- PAC visual editor
