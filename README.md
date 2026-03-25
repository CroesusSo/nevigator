# PAC-Enabled Proxy with TUN Support

A high-performance proxy server that uses PAC (Proxy Auto-Config) scripts to route traffic through TUN devices (VPN) or SOCKS5 upstreams, with automatic fallback to system default route.

## Features

- HTTP CONNECT and SOCKS5 proxy support
- Native PAC rule engine (no JavaScript) for high-performance evaluation
- Route strategies: TUN (bind to TUN interface IP), SOCKS5 upstream, DIRECT (system default)
- Automatic fallback to DIRECT if chosen route fails
- Hot-reload PAC file via file watcher or admin API
- Web admin UI for editing, testing, and reloading PAC
- External proxy registry: define upstream SOCKS5 proxies with optional credentials; use PAC values `EXTERNAL <id>` or `DEFAULT` to route through them
- Lightweight, single binary
- **Full IPv6 support**: CONNECT and SOCKS5 work with IPv6 destinations (including bracketed literals)

## Build

```bash
go build -o proxy-pac ./cmd/proxy-pac
```

## Configuration

Create a config file (e.g., `config.yaml`):

```yaml
proxy:
  http_listen: 0.0.0.0:8080
  socks5_listen: 0.0.0.0:1080
  admin_listen: 127.0.0.1:9090
  pac_path: /etc/proxy.pac
  tun_device: tun0

pac:
  local_ip: ""            # auto from tun0
  enable_url_glob: false   # set to true to preserve URL-glob rules (not recommended; native engine does not support URL-glob)
  dns_timeout: 500ms
  eval_timeout: 200ms
  enable_dns_cache: true
  dns_max_entries: 100     # max DNS cache entries (0 = unlimited)
  dns_ttl: 5m              # TTL for DNS cache entries
  cache_enabled: true
  cache_max_size: 10000    # maximum number of entries
  cache_ttl: 10m           # time-to-live
  deepest_match: false     # use deepest/specific match semantics instead of first-match
```

Run:

```bash
./proxy-pac --config config.yaml
```

## Native Rule Engine

The proxy uses a native Go rule engine for PAC evaluation. This provides **>100× performance improvement** compared to a JavaScript interpreter, with uncached evaluation in sub‑microseconds. The engine only processes PAC files that contain structured rules in the `__PAC_RULES__` JSON variable.

The engine uses a **unified reversed-label suffix trie** (radix tree) for all match types:
- Exact host matches are stored as full paths in the trie
- Domain suffix matches traverse the trie in reverse (e.g., `.google.com` → `com` → `google`)
- Host glob patterns (`*`, `?`) are compiled to regex and evaluated at the relevant trie node
- Earliest rule index wins when multiple patterns match (first‑match semantics)

An LRU cache with configurable TTL stores recent host→route results, further reducing latency. Cache statistics are available via `/api/cache` and included in `/api/status`.

**Note:** The native engine does **not** support JavaScript execution; raw `FindProxyForURL` scripts are not evaluated. Only the structured rule format is supported.

### URL‑Glob Limitation

URL‑glob rules (`shExpMatch(url, ...)`) are **not supported** because the proxy only sees the hostname for CONNECT requests. Such rules are ignored with a warning log.

### Configuration

Cache settings can be adjusted under the `pac` section:

```yaml
pac:
  cache_enabled: true
  cache_max_size: 10000  # maximum number of entries
  cache_ttl: 10m          # time‑to‑live
```

If `cache_enabled` is true but `cache_max_size` or `cache_ttl` are not set, defaults are 10000 entries and 10 minutes.

### Deepest Match Semantics

By default, the native rule engine selects the rule with the **lowest index** when multiple rules match (first‑match semantics). Setting `deepest_match: true` changes the matching behavior to prefer the **most specific** rule:

- **Domain suffix**: longest matching suffix wins (e.g., `.google.com` beats `.com`)
- **Host glob**: patterns with higher specificity score (longer, fewer wildcards) are evaluated first

Exact matches are unaffected. This feature is useful when you want intuitive "most specific" routing without manually reordering rules. It is **opt‑in** and disabled by default for backward compatibility.

Example:

```yaml
pac:
  deepest_match: true
```

With rules:

| Index | Pattern          | Route   |
|-------|------------------|---------|
| 0     | `.com`           | SOCKS5  |
| 1     | `.google.com`    | TUN     |

For `mail.google.com`:
- `deepest_match: false` → rule 0 wins (lower index) → SOCKS5
- `deepest_match: true`  → rule 1 wins (longer suffix) → TUN

## Admin UI

Open `http://localhost:9090` in browser to:
- View/edit PAC file
- Save changes and reload
- Test a URL to see which route is chosen
- View status
- **Proxies tab**: manage external SOCKS5 proxies, set default, test connectivity
- **Settings tab**: view and edit listening addresses, PAC path, and TUN device

## GFW List Integration

The proxy can optionally fetch and merge rules from the GFWlist. Configure under `gfw:` in config.yaml:

```yaml
gfw:
  disabled: false          # Set true to disable fetching and inclusion (default false)
  source_url: "https://..."
  local_rules_path: "..."
  user_rules_path: "..."
  fetch_on_startup: true
  update_interval: 24h
  http_timeout: 60s
```

- **disabled**: When true, the fetcher will not perform any fetches (startup, periodic, manual) and the PAC generator will exclude GFW rules. The feature can be toggled at runtime via the Admin UI (GFW tab).
- The Admin UI also allows manual fetching and displays status (enabled/disabled, last fetch, rule count).

## SSH Tunnels

The proxy supports SSH tunnels as an external proxy type alongside SOCKS5. This allows routing traffic through SSH servers using direct channel multiplexing.

### Performance Characteristics

The SSH tunnel implementation uses a lock-free worker pool design optimized for high concurrency:

- **Hot path**: `Dial` has no mutex contention; uses atomic state checks and non-blocking task enqueue
- **Worker pool**: Default 8 workers process dial tasks concurrently; each uses atomic load of the SSH client
- **Backpressure**: Buffered task channel (size 100) prevents unbounded memory growth; `Dial` returns `ErrOverload` immediately when saturated
- **Reconnection**: Dedicated manager goroutine handles connection lifecycle with exponential backoff; workers signal failures via non-blocking `failCh`
- **Graceful shutdown**: Context cancellation + WaitGroup ensures all workers exit cleanly without panics

Compared to the previous mutex-based implementation, this design significantly reduces tail latency and improves throughput under heavy HTTPS request load while maintaining API compatibility and existing behavior.

### Configuration

Add an external proxy with `kind: "ssh"`:

```yaml
proxy:
  external_proxies:
    - id: "ssh-gfw"
      kind: "ssh"
      address: "user@ssh.example.com:22"
      auth_method: "key"
      private_key_path: "/path/to/id_rsa"
      host_key_check: "strict"
```

- `address`: may include `user@` prefix and/or `:port`. The port can also be set via `ssh_port` (takes precedence). Default port 22.
- `username`: SSH username (optional if included in `address`)
- `auth_method`: `"password"` or `"key"` (required)
- For `"password"`: `password` field required
- For `"key"`: `private_key_path` required (unencrypted PEM)
- `host_key_check`: `"strict"` (default) or `"ignore"`. With `"strict"`, the system uses `~/.ssh/known_hosts` or a provided `host_key_fingerprint` (hex) for pinning.

### Keepalive (Heartbeat)

The SSH tunnel supports an optional keepalive heartbeat to detect silent connection failures. When enabled, the tunnel periodically sends SSH global keepalive requests to the server. If the server fails to respond after 3 consecutive attempts, the connection is closed and automatically reconnected.

Configuration in `config.yaml` under the external proxy's `keepalive` field:

```yaml
proxy:
  external_proxies:
    - id: "ssh-keepalive"
      kind: "ssh"
      address: "user@ssh.example.com:22"
      auth_method: "password"
      password: "secret"
      keepalive:
        enabled: true
        interval: 30s
```

- `enabled`: Turn keepalive on or off. Disabled by default.
- `interval`: How often to send keepalive probes. Default is `30s` when keepalive is enabled and interval is not set or is zero.

The keepalive uses SSH's `"keepalive@openssh.com"` global request with a per-ping timeout of 5 seconds. It runs in a dedicated goroutine per tunnel and does not block worker dial operations.

The keepalive settings are also available in the **Admin UI** → Proxies tab when creating or editing an SSH tunnel.

The admin UI's Proxies tab supports creating and testing SSH proxies. See the [External Proxy Registry spec](../openspec/specs/external-proxy-registry/spec.md) for full details.

## PAC Script Format

**Important:** This proxy uses a **native rule engine only** — it does **not** execute arbitrary JavaScript PAC functions. PAC files must contain a special `__PAC_RULES__` JSON variable with structured rules. The `FindProxyForURL` function is required but ignored; it's just a placeholder to maintain compatibility with the PAC file format.

Rules are typically created and edited through the **Web Admin UI**, which generates the correct `__PAC_RULES__` structure automatically. You can also edit the PAC file directly as JSON.

### Rule Structure

Each rule in `__PAC_RULES__` has:

- `id` – unique identifier
- `match` – match type: `exact`, `domain_suffix`, `host_glob` (URL-glob not supported)
- `value` – pattern to match (e.g., `"example.com"`, `".google.com"`, `"*.onion"`)
- `route` – one of: `"TUN"`, `"SOCKS5 host:port"`, `"EXTERNAL <id>"`, `"DEFAULT"`, `"DIRECT"`
- `enabled` – boolean

### Example PAC File

```javascript
var __PAC_RULES__ = {"version":1,"rules":[
  {"id":"1","match":"exact","value":"example.com","route":"TUN"},
  {"id":"2","match":"domain_suffix","value":".google.com","route":"SOCKS5 127.0.0.1:9050"},
  {"id":"3","match":"host_glob","value":"*.onion","route":"DIRECT"}
]};
function FindProxyForURL(url, host) { return 'DIRECT'; }
```

**External Proxies** are defined in `config.yaml` under `proxy.external_proxies`. Each has an `id`, `address`, and optional `username`/`password`. Use `"EXTERNAL <id>"` to route through them.

### Configuration Options

Under the `pac` section in `config.yaml`:

- `deepest_match: false` – When true, prefer the most specific suffix/glob instead of lowest index
- `cache_enabled: true` – Enable result caching
- `cache_max_size: 10000` – Max cached hosts
- `cache_ttl: 10m` – TTL for cached entries
- `enable_dns_cache: true` – Enable DNS lookup caching (for `dnsResolve` not used in native mode but retained for compatibility)
- `dns_max_entries: 100` – Max DNS cache entries
- `dns_ttl: 5m` – DNS TTL

For full details, see the [Native Rule Engine](#native-rule-engine) section.

## How It Works

1. Client connects via HTTP CONNECT or SOCKS5
2. Proxy asks PAC evaluator for route based on destination host
3. Router establishes outbound connection according to route:
   - TUN: binds socket to TUN interface IP
   - SOCKS5: connects to upstream and performs SOCKS5 handshake
   - DIRECT: normal net.Dial (uses OS routing)
4. If route fails (except DIRECT), automatically retries with DIRECT
5. Data is relayed between client and outbound

## Deployment

### Systemd

See `contrib/proxy-pac.service`.

### Docker

The container needs access to the host's TUN device if you want to use TUN routes. The proxy only needs to read the interface IP; it does not manipulate the TUN directly. Mount the config and PAC files as volumes.

## Testing

Unit tests: `go test ./...`

Manual test with curl:

```bash
curl -x http://localhost:8080 http://example.com
```

Or SOCKS5:

```bash
curl --socks5 localhost:1080 http://example.com
```

## Limitations

- TCP only (no UDP)
- No upstream HTTP proxy support (only SOCKS5)
- No client authentication
- PAC evaluation per request creates a fresh JS VM (~100-200µs overhead)

## License

MIT
