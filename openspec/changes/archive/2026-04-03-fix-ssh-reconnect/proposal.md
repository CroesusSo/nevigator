## Why

The SSH tunnel reconnection mechanism is broken. When an SSH connection drops (due to network blip, server restart, or keepalive timeout), existing and new connection requests immediately fail with "use of closed network connection" and the router falls back to DIRECT. Instead, the system should automatically reconnect with exponential backoff, and pending `Dial` calls should block waiting for reconnection to succeed, as specified in `openspec/specs/ssh-tunnel-proxy/spec.md` (lines 37-41, 111-124).

## What Changes

Implementation fixes in `ssh/tunnel.go`:

- Remove guard in `connect()` that blocks reconnection when state is "connected"
- Change manager to always call `connect()` on failure signals (remove `isConnected()` check)
- When keepalive detects failure: close client, set `client = nil` via atomic swap, and signal `failCh`
- When worker detects `client.Dial` error: set `client = nil` via atomic swap, signal `failCh`
- Workers wait on condition variable when `client == nil` instead of immediately returning errors
- Ensure `setClient()` safely closes old client after swapping

No breaking changes; purely bug fixes to match existing spec requirements.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None. The existing `ssh-tunnel-proxy` spec already defines the required reconnection behavior. This change fixes the implementation to satisfy those requirements.

## Impact

**Modified code:**
- `ssh/tunnel.go`: Tunnel.connect(), Tunnel.manager(), Tunnel.keepaliveLoop(), Tunnel.worker(), Tunnel.setClient()

**Tests:**
- New test: simulate SSH connection loss and verify automatic reconnection with backoff
- New test: verify pending Dial calls block during reconnection
- New test: verify keepalive-triggered reconnection signals manager and sets client=nil
- Existing tests must continue to pass

**No API or configuration changes.** Affects only internal tunnel management.
