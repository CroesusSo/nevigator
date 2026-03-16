## 1. Create `status` package with shared types and interface

- [x] 1.1 Create `status/status.go` with `Connection` struct (ID, ClientAddr, DestAddr, Route, Protocol, StartedAt)
- [x] 1.2 Add `ServerStatus` struct to `status/status.go` (HTTPListen, SOCKS5Listen, AdminListen, TUNDevice, TUNIP, TUNUp, ActiveConns, TotalConns)
- [x] 1.3 Add `StatusProvider` interface to `status/status.go` with `Status() ServerStatus` and `Connections() []Connection`

## 2. Add connection tracking to `proxy.Server`

- [x] 2.1 Add `sync.RWMutex`, `conns map[string]*Connection`, `totalConns atomic.Int64`, `nextID atomic.Int64` fields to `Server` struct
- [x] 2.2 Add `trackConnection(conn net.Conn, dest, route, protocol string) string` method that creates and stores a `Connection`
- [x] 2.3 Add `untrackConnection(id string)` method that removes a connection from the map
- [x] 2.4 Modify `handleHTTPConnect` to track the connection with `protocol: "http"` before calling `relay`
- [x] 2.5 Modify `handleSOCKS5` to track the connection with `protocol: "socks5"` before calling `relay`
- [x] 2.6 Modify `relay` to accept a connection ID parameter and defer `untrackConnection`
- [x] 2.7 Implement `Status() status.ServerStatus` on `Server` — gather listen addresses, re-check TUN interface, return counts
- [x] 2.8 Implement `Connections() []status.Connection` on `Server` — snapshot current connections from map under read lock

## 3. Add API endpoints to `admin.Handler`

- [x] 3.1 Add `statusProvider status.StatusProvider` field to `Handler` struct
- [x] 3.2 Update `NewHandler` to accept `status.StatusProvider` parameter
- [x] 3.3 Rewrite `handleStatus` to return `ServerStatus` JSON from `statusProvider.Status()`
- [x] 3.4 Add `handleConnections` that returns `[]Connection` JSON from `statusProvider.Connections()`
- [x] 3.5 Register `GET /api/connections` route in `RegisterRoutes`

## 4. Update admin UI

- [x] 4.1 Add Summary tab HTML panel between header and existing tabs
- [x] 4.2 Add Connections tab HTML panel
- [x] 4.3 Add JavaScript `loadStatus()` function that fetches `/api/status` and renders listening addresses, TUN status, connection counts
- [x] 4.4 Add JavaScript `loadConnections()` function that fetches `/api/connections` and renders the connection table
- [x] 4.5 Add `setInterval(loadStatus, 5000)` for Summary auto-refresh
- [x] 4.6 Add `setInterval(loadConnections, 2000)` for Connections auto-refresh
- [x] 4.7 Update tab switching logic to include Summary and Connections tabs
- [x] 4.8 Add CSS for status indicator (up/down dot), connection table styling

## 5. Wire `StatusProvider` in `main.go`

- [x] 5.1 Pass `proxySrv` as `status.StatusProvider` to `admin.NewHandler`
- [x] 5.2 Update `admin.NewHandler` call signature with the new parameter

## 6. Tests

- [x] 6.1 Add unit test for `status` package types (struct initialization, interface satisfaction)
- [x] 6.2 Add unit test for `proxy.Server` connection tracking (track, untrack, status snapshot)
- [x] 6.3 Add unit test for `admin.handleStatus` endpoint with mock `StatusProvider`
- [x] 6.4 Add unit test for `admin.handleConnections` endpoint with mock `StatusProvider`
- [x] 6.5 Run `go test ./...` and verify all tests pass
- [x] 6.6 Run `go vet ./...` and verify no issues
