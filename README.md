# PAC-Enabled Proxy with TUN Support

A high-performance proxy server that uses PAC (Proxy Auto-Config) scripts to route traffic through TUN devices (VPN) or SOCKS5 upstreams, with automatic fallback to system default route.

## Features

- HTTP CONNECT and SOCKS5 proxy support
- PAC script evaluation via goja JavaScript runtime
- Route strategies: TUN (bind to TUN interface IP), SOCKS5 upstream, DIRECT (system default)
- Automatic fallback to DIRECT if chosen route fails
- Hot-reload PAC file via file watcher or admin API
- Web admin UI for editing, testing, and reloading PAC
- External proxy registry: define upstream SOCKS5 proxies with optional credentials; use PAC values `EXTERNAL <id>` or `DEFAULT` to route through them
- Lightweight, single binary

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
  dns_timeout: 500ms
  eval_timeout: 200ms
   enable_dns_cache: true
   cache_enabled: true
   cache_max_size: 10000
   cache_ttl: 10m
```

Run:

```bash
./proxy-pac --config config.yaml
```

## Native Rule Engine

For generated PAC files (containing `__PAC_RULES__`), the proxy uses a native Go rule engine instead of the JavaScript interpreter. This provides **>100× performance improvement** for uncached evaluation (sub‑microsecond vs millisecond). The engine builds efficient indexes:

- Exact host map (O(1))
- Reversed‑label domain suffix trie (O(depth))
- Deduplicated, ordered host glob patterns

An LRU cache with configurable TTL stores recent host→route results, further reducing latency. Cache statistics are available via `/api/cache` and included in `/api/status`.

### URL‑Glob Limitation

URL‑glob rules (`shExpMatch(url, ...)`) are **not supported** in native mode because the proxy only sees the hostname for CONNECT requests. Such rules are ignored with a warning log, same as in JS mode.

### Configuration

Cache settings can be adjusted under the `pac` section:

```yaml
pac:
  cache_enabled: true
  cache_max_size: 10000  # maximum number of entries
  cache_ttl: 10m          # time‑to‑live
```

If `cache_enabled` is true but `cache_max_size` or `cache_ttl` are not set, defaults are 10000 entries and 10 minutes.

## Admin UI

Open `http://localhost:9090` in browser to:
- View/edit PAC file
- Save changes and reload
- Test a URL to see which route is chosen
- View status
- **Proxies tab**: manage external SOCKS5 proxies, set default, test connectivity
- **Settings tab**: view and edit listening addresses, PAC path, and TUN device

## PAC Script

PAC must define `FindProxyForURL(url, host)` and return one of:
- `"TUN"` - route via TUN device
- `"SOCKS5 host:port"` - route via SOCKS5 upstream
- `"EXTERNAL <id>"` - route via an external proxy defined in the main configuration
- `"DEFAULT"` - use the default external proxy (if configured)
- `"DIRECT"` - system default route

**External Proxies** are defined in the main `config.yaml` under `proxy.external_proxies`. Each has an `id`, `address`, and optional `username`/`password`.

Example:

```javascript
function FindProxyForURL(url, host) {
  if (dnsDomainIs(host, ".internal.example")) {
    return "TUN";
  }
  if (shExpMatch(host, "*.onion")) {
    return "SOCKS5 127.0.0.1:9050";
  }
  return "DIRECT";
}
```

## How It Works

1. Client connects via HTTP CONNECT or SOCKS5
2. Proxy asks PAC evaluator for route based on destination host
3. Router establishes outbound connection according to route:
   - TUN: binds socket to TUN interface IP
   - SOCKS5: connects to upstream and performs SOCKS5 handshake
   - DIRECT: normal net.Dial (uses OS routing)
4. If route fails (except DIRECT), automatically retries with DIRECT
5. Data is relayed between client and outbound

## Supported PAC Built-ins

- `shExpMatch(str, pattern)`
- `dnsDomainIs(domain, pattern)`
- `isInNet(ip, pattern, mask)`
- `myIpAddress()` (returns configured TUN IP)
- `dnsResolve(hostname)` (with timeout and cache)
- `isResolvable(hostname)`

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
