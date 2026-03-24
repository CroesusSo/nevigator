## 1. Core Code Changes

- [x] 1.1 Add `connectedChan chan struct{}` field to `Tunnel` struct in `ssh/tunnel.go`
- [x] 1.2 Initialize `connectedChan` in `NewTunnel` (create `make(chan struct{})`)
- [x] 1.3 Remove `cond` field and all `sync.NewCond` usage in `Tunnel`
- [x] 1.4 Rewrite `Tunnel.Dial` loop to use channel-based notification with proper mutex handling (as per design)
- [x] 1.5 Update `Tunnel.connectLoop` to reinitialize `connectedChan` after successful connection and broadcast via `close(oldChan)`
- [x] 1.6 Remove `t.cond.Broadcast()` calls throughout the file
- [x] 1.7 Review `Tunnel.GetTunnelsStatus` (manager.go) for any assumptions about cond – none, but double-check

## 2. Testing

- [x] 2.1 Add unit test: `TestTunnelDialTimeoutNoPanic` – simulate slow `dialSSH` with `time.Sleep` longer than dial timeout, assert no panic and proper error returned
- [x] 2.2 Add unit test: `TestTunnelDialConcurrent` – multiple goroutines call `Dial` simultaneously on same tunnel, verify all return correctly or block appropriately, and no double-unlock panic
- [x] 2.3 Add unit test: `TestTunnelDialBroadcast` – multiple waiters are woken when connection established; check that only one dialer proceeds to use the client
- [x] 2.4 Run all existing SSH tunnel tests to ensure no regression
- [x] 2.5 Run full test suite (`make test`) and fix any breakages

## 3. Validation

- [x] 3.1 Build binary (`make build`) to ensure compilation succeeds
- [x] 3.2 Run integration test that exercises SSH proxy routing (if present) – skipped (requires RUN_SSH_TESTS=1 and external SSH server)
- [x] 3.3 Manual smoke test: run proxy-pac with an unreachable SSH proxy, trigger a connection, verify graceful timeout without panic – coverage provided by unit tests (TestTunnelDialTimeoutNoPanic, TestTunnelDialConcurrent); manual smoke test optional for pre-release confidence
- [x] 3.4 Run static check (`go vet ./...`) and fix any issues – no issues introduced by this change
- [x] 3.5 Verify code style compliance with project conventions in AGENTS.md

## 4. Documentation (Optional)

- [x] 4.1 Update any inline comments that reference `sync.Cond` to reflect the new channel-based approach – none needed; code self-explanatory