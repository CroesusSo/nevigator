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

## ADDED Requirements

### Requirement: External Proxy Registry
The system SHALL maintain a registry of external SOCKS5 proxies defined in configuration and manageable via admin UI and API. Each proxy entry SHALL include:
- `id`: unique string identifier (non-empty, not reserved)
- `address`: valid `host:port` string
- `username` (optional)
- `password` (optional)

The registry SHALL be used by the router to resolve `EXTERNAL <id>` and `DEFAULT` routing strategies.

#### Scenario: Add new external proxy via API
- **WHEN** an administrator POSTs `/api/proxies` with valid JSON body containing `id`, `address`, and optional credentials
- **THEN** the proxy SHALL be added to the registry
- **AND** the config file SHALL be updated atomically
- **AND** the router's internal registry SHALL be updated immediately

#### Scenario: List external proxies (no credentials)
- **WHEN** an administrator GETs `/api/proxies`
- **THEN** the response SHALL be a JSON array of objects containing `id`, `address`, and `has_auth` (boolean), without `password`

#### Scenario: Update external proxy
- **WHEN** an administrator PUTs `/api/proxies/:id` with updated fields
- **THEN** the existing proxy SHALL be updated (immutable `id` cannot be changed)
- **AND** if `password` is omitted, the existing password SHALL be preserved
- **AND** the registry SHALL be updated

#### Scenario: Delete external proxy
- **WHEN** an administrator DELETE `/api/proxies/:id`
- **THEN** the proxy SHALL be removed from the registry
- **AND** any rules referencing it SHALL produce `EXTERNAL <id>` in PAC but will fail at routing time (fallback to DIRECT)

#### Scenario: Test connectivity to an unsaved proxy
- **WHEN** an administrator POSTs `/api/proxies/test` with address and credentials
- **THEN** the system SHALL attempt a TCP connection followed by SOCKS5 greeting and optional username/password authentication
- **AND** return `{ "success": true }` on successful handshake, or `{ "success": false, "error": "..."} ` on failure

#### Scenario: Test connectivity to a saved proxy
- **WHEN** an administrator POSTs `/api/proxies/:id/test`
- **THEN** the system SHALL load the stored proxy definition from config
- **AND** perform the same connectivity test
- **AND** return the result

#### Scenario: Preventing loops
- **WHEN** an external proxy address uses `127.0.0.1`, `localhost`, or `::1` and its port matches any of this proxy's listening ports (HTTP, SOCKS5, Admin)
- **THEN** the API SHALL reject the request with HTTP 400 and appropriate error message
- **AND** the config SHALL NOT be updated

#### Scenario: Reserved ID validation
- **WHEN** an external proxy `id` is `"DIRECT"`, `"TUN"`, or `"DEFAULT"`
- **THEN** the API SHALL reject the request with HTTP 400

#### Scenario: Duplicate ID validation
- **WHEN** an external proxy with an `id` that already exists in the registry is created (or an update tries to change ID to an existing one)
- **THEN** the API SHALL reject the request with HTTP 409 Conflict

### Requirement: Default External Proxy
The system SHALL allow administrators to designate one registered external proxy as the default via `proxy.default_external_proxy` in configuration or via admin API. The special PAC return value `"DEFAULT"` SHALL instruct the router to use this default proxy. Additionally, the PAC generator SHALL use `"DEFAULT"` for rules with empty route when a default proxy is configured (falling back to legacy hardcoded SOCKS5 when no default is set).

#### Scenario: Router uses default proxy
- **WHEN** `Evaluator.Evaluate(host)` returns `Route{Strategy: "DEFAULT"}`
- **THEN** the router SHALL look up the configured `default_external_proxy` ID in the registry
- **AND** establish a SOCKS5 connection through that proxy
- **IF** the default proxy is not configured or not found, the router SHALL return an error, causing `Route` to fallback to DIRECT

#### Scenario: PAC generator emits DEFAULT
- **GIVEN** `proxy.default_external_proxy` is set to `"tor"`
- **WHEN** `gfw.GeneratePAC` is called with a rule having empty `Route`
- **THEN** the generated PAC SHALL contain `return "DEFAULT";` for that rule
- **IF** no default is set, the generator SHALL use `return "SOCKS5 127.0.0.1:1080";` (legacy behavior)

#### Scenario: Changing default proxy triggers PAC regeneration
- **WHEN** an administrator updates `default_external_proxy` via `/api/config`
- **THEN** the router's default proxy ID SHALL be updated immediately
- **AND** the PAC file SHALL be regenerated so that GFW rules (with empty route) now point to `DEFAULT`
