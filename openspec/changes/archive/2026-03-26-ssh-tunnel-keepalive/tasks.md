## 1. Configuration and Types

- [x] 1.1 Add `KeepaliveConfig` struct to `pac/config.go`: `Enabled bool`, `Interval time.Duration`
- [x] 1.2 Add `Keepalive KeepaliveConfig` field to `ExternalProxy` struct
- [x] 1.3 Implement default values: when unmarshaling YAML/JSON, if `keepalive` is present but `interval` is zero, default to 30s; treat missing `keepalive` as disabled
- [x] 1.4 Add JSON/YAML struct tags: `yaml:"keepalive" json:"keepalive"`
- [x] 1.5 Ensure backward compatibility: if `keepalive` field is absent in config, treat as `KeepaliveConfig{Enabled: false, Interval: 0}`

## 2. Tunnel Keepalive Implementation

- [x] 2.1 Extend `Tunnel` struct (`ssh/tunnel.go`) with fields:
  - `keepaliveCtx context.Context`
  - `keepaliveCancel context.CancelFunc`
  - `keepaliveFailures int32` (atomic)
- [x] 2.2 Implement `startKeepalive(client *ssh.Client)` method:
  - Create `keepaliveCtx, cancel = context.WithCancel(t.ctx)`
  - Launch goroutine: `t.wg.Add(1); go func(){ defer t.wg.Done(); t.keepaliveLoop(keepaliveCtx, client) }()`
  - Log "keepalive started for tunnel {id} with interval {cfg.Keepalive.Interval}"
- [x] 2.3 Implement `stopKeepalive()` method:
  - If `keepaliveCancel != nil`, call `keepaliveCancel()`
  - Reset fields (`keepaliveCancel = nil`)
  - Log "keepalive stopped for tunnel {id}"
- [x] 2.4 Modify `setClient(newClient *ssh.Client)`:
  - Call `stopKeepalive()` before swapping client
  - After successful swap, if `t.cfg.Keepalive.Enabled` and `newClient != nil`, call `startKeepalive(newClient)`
- [x] 2.5 Implement `keepaliveLoop(ctx context.Context, client *ssh.Client)`:
  - Create ticker with `t.cfg.Keepalive.Interval`
  - Loop: select on `ticker.C` and `ctx.Done()`
    - For each tick: create child context `context.WithTimeout(ctx, 5s)`, launch goroutine that sends `client.SendRequest("keepalive@openssh.com", true, nil)` on error channel, parent selects with timeout to avoid goroutine leaks
    - On error (including timeout): log warning, increment `keepaliveFailures`; on success: reset to 0
    - If `keepaliveFailures >= 3`: log error, call `client.Close()`, return
- [x] 2.6 Ensure `keepaliveFailures` resets to 0 when starting fresh (in `startKeepalive` via atomic store before starting)
- [x] 2.7 Verify `Close()` method stops keepalive: add explicit `t.stopKeepalive()` call after `t.cancel()` to clean up fields

## 3. TCP Keepalive Setup

- [x] 3.1 Modify `dialSSH(cfg pac.ExternalProxy)` function to use custom `net.Dialer`:
```go
dialer := &net.Dialer{
    Timeout:   30 * time.Second,
    KeepAlive: 30 * time.Second,
}
conn, err := dialer.Dial("tcp", addr)
```
- [x] 3.2 On success, proceed with `ssh.NewClientConn(conn, addr, sshCfg)` as before
- [x] 3.3 Ensure error handling: if `ssh.NewClientConn` fails, close `conn`

## 4. Router and Tunnel Manager Integration

- [x] 4.1 Verify `Tunnel` receives full `ExternalProxy` config (including keepalive) from `Manager.getOrCreateTunnel()` → already passes `cfg`
- [x] 4.2 Test that when keepalive config changes, the tunnel is recreated: `Manager.UpdateProxies()` closes old tunnel; new tunnel reads new config
- [x] 4.3 Confirm that `keepalive.Enabled == false` results in no keepalive goroutine (no side effects)
- [x] 4.4 Add log messages: "keepalive started", "keepalive failed: X consecutive failures", "keepalive stopped"

## 5. Admin UI and API

- [ ] 5.1 Extend API proxy schema in `admin/handlers.go`:
  - PATCH/PUT `/api/proxies/{id}` accepts JSON body with optional `keepalive` object containing `enabled` (bool) and `interval` (duration string)
  - Validation: if `enabled` true, `interval` must be > 0; reject invalid durations
  - Convert duration strings to `time.Duration` before storing in config
- [ ] 5.2 Update GET `/api/proxies` to include keepalive settings in response JSON
- [ ] 5.3 Modify admin UI HTML/JS (inline in `handlers.go`):
  - In proxy edit form, add keepalive section: checkbox "Enable keepalive", input for interval (with unit dropdown or duration format)
  - Pre-fill values when editing existing proxy
   - Help text: "Sends SSH keepalive probes every interval (default 30s) with 5s timeout; after 3 failures triggers reconnection."
- [ ] 5.4 Update proxy creation modal/dialog similarly
- [ ] 5.5 Ensure proxy list page shows keepalive status (e.g., "enabled (30s)" or "disabled")

## 6. Testing

- [ ] 6.1 Unit test `KeepaliveConfig` defaults: zero values → disabled; interval parsing from YAML/JSON
- [ ] 6.2 Unit test `startKeepalive`: with `Enabled=true`, verify goroutine starts; ticker fires; `SendRequest` called; with `Enabled=false`, verify no goroutine starts
- [ ] 6.3 Unit test `keepaliveLoop` using mocked SSH client (interface or test double):
  - Simulate sequence: success → success → error → error → error → triggers `client.Close()`
   - Simulate timeout after 5s -> counts as failure
  - Verify failures counter resets on success
- [ ] 6.4 Unit test `stopKeepalive`: ensure cancellation works, goroutine exits, no leaks
- [ ] 6.5 Integration test: start real SSH server (or use `ssh.NopCloser` test server) that stops responding after keepalive establishes; verify tunnel reconnects after 3 failed keepalives
- [ ] 6.6 Run existing SSH tunnel tests with `-race` to ensure no new data races
- [ ] 6.7 Regression test: existing test suite must pass unchanged

## 7. Documentation and Finalization

- [ ] 7.1 Update `config.yaml.example` to include keepalive example under an SSH proxy
```yaml
proxy:
  external_proxies:
    - id: "ssh-gfw"
      kind: "ssh"
      address: "ssh.example.com:2222"
      auth_method: "key"
      private_key_path: "/path/to/id_rsa"
      keepalive:
        enabled: true
        interval: 30s
```
- [ ] 7.2 Add section to README.md about SSH keepalive: how to enable, defaults, troubleshooting
- [ ] 7.3 Code comments: document `keepaliveLoop` behavior, timeouts, failure threshold
- [ ] 7.4 Run `go fmt ./...`, `go vet`, `golangci-lint` if available
- [ ] 7.5 Build binary and perform manual smoke test with real SSH server (optional but recommended)
