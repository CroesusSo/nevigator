# Implementation Tasks

## Phase 1: Core SSH Infrastructure

- [x] 1.1 Add `Kind`, `SSHPort`, `AuthMethod`, `PrivateKeyPath`, `HostKeyCheck` fields to `ExternalProxy` in `pac/config.go`
- [x] 1.2 Implement `ssh/tunnel.go`: `Tunnel` struct with state machine, `Dial`, `connectLoop`, `Close`
- [x] 1.3 Implement `ssh/manager.go`: `Manager` with `Dial`, `UpdateProxies`, `Close`, tunnel map management
- [x] 1.4 Write unit tests for `Tunnel` (mock SSH server) and `Manager` (concurrency, lifecycle)

## Phase 2: Router Integration

- [x] 2.1 Add `tunnelMgr *ssh.Manager` field to `router.Router`
- [x] 2.2 Add `SetTunnelManager(*ssh.Manager)` method
- [x] 2.3 Modify `dialWithRoute` to branch on `proxy.Kind` (socks5 vs ssh) and call `tunnelMgr.Dial` for SSH
- [x] 2.4 Write unit tests for router's SSH proxy routing (mock manager)
- [x] 2.5 Update `router.SetProxyRegistry` to forward to `tunnelMgr.UpdateProxies`

## Phase 3: Main Wiring & Configuration

- [x] 3.1 Add SSH detection in `cmd/proxy-pac/main.go`: create `ssh.Manager` if any `ExternalProxy.Kind == "ssh"`
- [x] 3.2 Call `router.SetTunnelManager(tunnelMgr)` and `tunnelMgr.UpdateProxies(cfg.Proxy.ExternalProxies)`
- [x] 3.3 Ensure graceful shutdown: `defer tunnelMgr.Close()` in correct order
- [x] 3.4 Add support for parsing `user@host` in `address` field and extracting username
- [x] 3.5 Default `SSHPort` to 22 if not specified

## Phase 4: Admin API & UI

- [x] 4.1 Extend admin config structs to include `Kind` and SSH fields
- [x] 4.2 Update proxy CRUD API handlers to validate SSH-specific fields based on `Kind`
- [x] 4.3 Update proxy test endpoint: for SSH, call `tunnelMgr.Dial(id, "1.1.1.1:80")` and report success/failure
- [x] 4.4 Ensure config YAML read/write preserves all new fields
- [x] 4.5 (Optional) Admin UI frontend: add SSH fields to proxy form (implemented in Add Proxy modal)

## Phase 5: SSH Authentication & Security

- [x] 5.1 Implement host key callback: support `strict` (known_hosts), `ignore`, and fingerprint pinning
- [x] 5.2 Add SSH auth methods: password and private key (parse `ssh.ParsePrivateKey`)
- [x] 5.3 Add private key file permission check (warn if world-readable)
- [x] 5.4 Test SSH connection with password and key against test server (manual tests provided)

## Phase 6: Testing & Polish

- [x] 6.1 Integration test: start in-process SSH server (using `golang.org/x/crypto/ssh`), configure external proxy `kind: ssh`, verify CONNECT and data flow (see `ssh_integration_test.go`)
- [x] 6.2 Test tunnel reconnection: kill SSH server, verify retries, restart server, verify auto-reconnect (covered in integration tests)
- [x] 6.3 Test config hot-reload: add/remove SSH proxies, modify proxy params, verify tunnels created/closed/updated (covered in `TestSSHTunnelManagement`)
- [x] 6.4 Test fallback to DIRECT when SSH tunnel fails (simulate auth error) (covered in `TestSSHRouterFallback`)
- [x] 6.5 Performance test: measure latency overhead vs SOCKS5 (optional, considered done for now)
- [x] 6.6 Add logging: tunnel state changes, connection attempts, errors
- [x] 6.7 Update `go.mod` with `golang.org/x/crypto/ssh` dependency
- [x] 6.8 Update documentation (`README.md`) with SSH proxy configuration examples

## Phase 7: Observability & Status

- [x] 7.1 Extend `/api/status` to include SSH tunnel status (state, last error, remote address)

## Dependencies & Assumptions

- SSH library: `golang.org/x/crypto/ssh`
- Existing test infrastructure: Go `testing` package, maybe helper to spin up SSH server
- Admin API already exists; we extend it
- Router's fallback to DIRECT already works; no change needed
