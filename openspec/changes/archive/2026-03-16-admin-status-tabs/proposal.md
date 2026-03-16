## Why

The admin UI currently shows routing rules, GFW list management, and URL testing, but provides no visibility into the proxy server's runtime state. Operators cannot see whether the TUN device is up, what addresses are listening, or how many connections are active. This makes debugging routing issues and monitoring load difficult without checking logs or running shell commands.

## What Changes

- Add a **Summary** tab to the admin UI showing:
  - Listening addresses for HTTP proxy, SOCKS5 proxy, and admin UI
  - Outgoing addresses: TUN device name, IP, and up/down status; SOCKS5 upstream info
  - Active connection count and total connections since startup
- Add a **Connections** tab to the admin UI showing a live table of active connections with client address, destination, route type, protocol, and duration
- New API endpoints:
  - `GET /api/status` — returns server summary (listening addresses, TUN status, connection counts)
  - `GET /api/connections` — returns list of active connections
- Introduce a `status` package with shared types (`Connection`, `ServerStatus`) and a `StatusProvider` interface to decouple the admin handler from the proxy server
- Add connection tracking to `proxy.Server` using a mutex-protected map with atomic counters

## Capabilities

### New Capabilities
- `admin-status`: Server summary display — listening addresses, TUN/SOCKS5 outgoing info, connection counts
- `admin-connections`: Active connection tracking and display — live table of connections with client, destination, route, and duration

### Modified Capabilities

## Impact

- New package: `status/` (3 exported types, 1 interface)
- Modified: `proxy/server.go` (connection tracking, StatusProvider implementation)
- Modified: `admin/handlers.go` (2 new endpoints, 2 new UI tabs, StatusProvider dependency)
- Modified: `cmd/proxy-pac/main.go` (wire StatusProvider into admin handler)
- No external dependencies added — uses only stdlib (`sync`, `sync/atomic`, `time`, `net`)
