# IPv6 Destination Handling Fix – Design

## Architecture Context

```
Client ── HTTP CONNECT ──> Proxy ── Route ──> Outbound
           (or SOCKS5)
```

The proxy receives a CONNECT request with a target host:port. After parsing, we need to produce a well‑formed destination address for:

- `net.Dial` (DIRECT, TUN)
- SOCKS5 CONNECT request to upstream proxy

The Go `net` package expects IPv6 literals to be bracketed (`[ipv6]:port`). When we parse with `SplitHostPort`, we lose brackets. Thus we must re‑add them when appropriate.

## New Helper: `formatDest`

```go
// formatDest constructs a destination address suitable for net.Dial and routing.
// It ensures IPv6 addresses (those containing colons and not already bracketed) are wrapped in brackets.
func formatDest(host string, port int) string {
    if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") && !strings.HasSuffix(host, "]") {
        return fmt.Sprintf("[%s]:%d", host, port)
    }
    return fmt.Sprintf("%s:%d", host, port)
}
```

**Logic:**
- If `host` contains a colon (likely IPv6) and is not already bracketed, add brackets.
- Otherwise (IPv4 or domain), leave unchanged.

## Changes in `proxy/server.go`

### 1. `handleHTTPConnect`

Before:

```go
host, portStr, err := net.SplitHostPort(r.Host)
...
port, _ := strconv.Atoi(portStr)
dest := fmt.Sprintf("%s:%d", host, port)
```

After:

```go
host, portStr, err := net.SplitHostPort(r.Host)
...
port, _ := strconv.Atoi(portStr)
dest := formatDest(host, port)
```

### 2. `handleHTTPSConnect`

Same pattern: ensure `dest` built via `formatDest`.

### 3. `handleSOCKS5`

Before:

```go
host, port, _, err := socks5.DeserializeAddr(reqBuf, 3)
...
dest := fmt.Sprintf("%s:%d", host, port)
```

After:

```go
host, port, _, err := socks5.DeserializeAddr(reqBuf, 3)
...
dest := formatDest(host, int(port))
```

### 4. `router.dialWithRoute` – No change needed if `dest` arrives correctly formatted. The `dialer.Dial("tcp", dest)` will now receive a proper address for IPv6.

## Data Flow

```
HTTP CONNECT:  r.Host = "[2001:db8::1]:443"
                split -> host="2001:db8::1", port=443
                formatDest -> "[2001:db8::1]:443"
                Router.Route(dest, route) -> net.Dial("tcp", dest) succeeds

SOCKS5 client: DeserializeAddr returns host="2001:db8::1", port=443
                formatDest -> "[2001:db8::1]:443"
                CONNECT request to upstream: socks5.SerializeAddr(host, port) -> correct ATYPIPv6
```

## Testing

- Unit tests for `formatDest` covering IPv6, IPv4, domain names, already-bracketed.
- Integration tests: HTTP CONNECT to IPv6 target, SOCKS5 CONNECT to IPv6 target via direct and upstream.

## Compatibility

- Backward compatible: formatting for IPv4 and domains unchanged.
- Existing bracketed IPv6 addresses pass through unchanged (condition checks for `[`).

## Implementation Notes

- `formatDest` should be a private function in `proxy/server.go` (or a small utility if needed elsewhere).
- No changes to PAC or native engine.
- No changes to configuration schema.

---

This design is straightforward and localized. It restores IPv6 destination support without altering existing routing logic.