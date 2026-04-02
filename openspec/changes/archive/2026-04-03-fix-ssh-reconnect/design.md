# Design: Fix SSH Tunnel Reconnection

## Problem

When an SSH tunnel connection drops (network blip, server restart, or keepalive timeout), new connection requests fail immediately with "use of closed network connection" and the router falls back to DIRECT. The tunnel does not automatically reconnect, violating spec requirements (ssh-tunnel-proxy.md:37-41, 111-124).

**Root Causes:**
1. `connect()` refuses to run if state is "connected" (guard too strict)
2. Manager only calls `connect()` when `isConnected()` returns true, but when client is nil (after failure) `isConnected()` returns false
3. Keepalive and workers do not mark the client as broken (`client = nil`) after detecting failure, so workers continue using a dead client
4. Failures signal `failCh` but state remains stale; workers that see a nil client already wait, but that condition never triggers because `client` never becomes nil

## Current Architecture (Simplified)

```
┌─────────────────────────────────────────┐
│          Tunnel                         │
│  ├─ Manager goroutine                   │
│  ├─ Worker pool (8)                     │
│  ├─ Atomic client (*ssh.Client)        │
│  ├─ State (closed/connecting/connected)│
│  ├─ dialCh (buffered)                  │
│  ├─ failCh (buffered)                  │
│  └─ cond (for waiting workers)         │
└─────────────────────────────────────────┘
```

## Proposed Solution

**Key principle:** Any failure (keepalive or worker) marks the tunnel as broken by atomically setting `client = nil`. The manager unconditionally attempts reconnection upon receiving `failCh`. Workers that encounter `client == nil` wait on the condition variable for reconnection.

### State Machine (No New States)

```
   closed ──connect()──> connecting ──success──> connected
                                   │
                                   │ failure (retry)
                                   ▼
                              (backoff, retry)
                                   │
                                   │ success
                                   ▼
                                connected
```

`reconnecting` from the spec is simply the `connecting` state after a failure.

### Changes Detail

#### 1. Manager: Always Reconnect on Failure Signals

**Before (router.go:144-148, but actually in ssh/tunnel.go manager):**
```go
case <-t.failCh:
    if t.isConnected() {
        t.connect()
    }
```

**After:**
```go
case <-t.failCh:
    t.connect()
```

**Rationale:** The `failCh` signal means something failed. We always want to attempt reconnection unless already connecting (guarded in `connect()`). The previous `isConnected()` check prevented reconnection when client was nil.

#### 2. `connect()` Guard: Allow Reconnection from "connected"

**Before:**
```go
current := t.state.Load().(string)
if current == stateConnecting || current == stateConnected {
    return
}
```

**After:**
```go
current := t.state.Load().(string)
if current == stateConnecting {
    return
}
```

**Rationale:** The manager calls `connect()` only when a failure occurs. Even if state is "connected", we need to transition to "connecting" and re-establish the SSH connection. Blocking on "connected" prevented all reconnection.

#### 3. Keepalive: Mark Tunnel Broken

**Before (tunnel.go:278-286):**
```go
if failures >= maxFailures {
    log.Printf("[SSH] Tunnel %s keepalive failed %d times; closing connection", t.proxyID, failures)
    client.Close() // Trigger reconnection via failCh
    return
}
```

**After:**
```go
if failures >= maxFailures {
    log.Printf("[SSH] Tunnel %s keepalive failed %d times; closing connection", t.proxyID, failures)
    // Mark tunnel as broken: close client, set pointer to nil, signal manager
    old := t.client.Swap(nil)
    if old != nil {
        old.Close()
    }
    select {
    case t.failCh <- struct{}{}:
    default:
    }
    return
}
```

**Rationale:** Setting `client = nil` makes future workers block instead of using a dead client. Signaling `failCh` wakes manager to reconnect.

#### 4. Worker: Mark Broken on Any `client.Dial` Error

**Before (tunnel.go:362-372):**
```go
conn, err := client.Dial("tcp", task.dest)
if err != nil {
    select {
    case t.failCh <- struct{}{}:
    default:
    }
    task.respChan <- dialResult{err: err}
    continue
}
```

**After:**
```go
conn, err := client.Dial("tcp", task.dest)
if err != nil {
    // Mark tunnel as broken: swap client to nil and close old
    old := t.client.Swap(nil)
    if old != nil {
        old.Close()
    }
    // Signal manager to reconnect (non-blocking)
    select {
    case t.failCh <- struct{}{}:
    default:
    }
    task.respChan <- dialResult{err: err}
    continue
}
```

**Rationale:** Ensures the first worker to detect the broken connection marks it as such, causing subsequent workers to wait. The original error is still returned to the caller (router will fallback to DIRECT).

#### 5. Worker Wait Logic (Already Present)

The worker code already contains the correct wait logic when `client == nil` (tunnel.go:348-360). No changes needed.

### Concurrency Guarantees

- `client` is `atomic.Pointer[ssh.Client]`; `Swap` and `Load` are atomic.
- Only the manager modifies state; workers read state only for debugging.
- `failCh` is buffered (size 10) to prevent blocking even if multiple failures occur before manager processes.
- `cond.Broadcast()` in `setClient()` wakes all waiting workers.
- `connect()` is called only by manager (single goroutine), so no race on concurrent connection attempts.

### Error Handling

- Permanent errors (password denied, host key mismatch) are still detected by `isPermanentError()` in `dialSSH`. `connect()` sets `closing=true` and stops retrying.
- `close()` sets `closing=true` and cancels context; manager and workers exit gracefully.
- Workers that see `closing` while waiting return `ErrTunnelClosed` immediately.

### Performance Impact

- One extra atomic `Swap` on failure paths (rare).
- Workers that hit the first failure still return error immediately (no added latency).
- Subsequent workers block briefly during reconnection (desired behavior).
- No additional locks in hot path (dial tasks).

### Testing Strategy

1. **Keepalive-triggered reconnection**
   - Create tunnel with keepalive enabled, force keepalive to fail (mock `client.SendRequest` to return error after N attempts)
   - Verify: after max failures, client becomes nil, `failCh` signaled, manager calls `connect()`, new client established, new keepalive started.

2. **Worker-detected failure**
   - Mock `client.Dial` to return "use of closed network connection" on first call, then succeed.
   - Call `Dial()`: expect error returned.
   - Call second `Dial()` concurrently: expect it blocks until reconnection succeeds, then returns success.
   - Assert no panics, proper state transitions.

3. **Reconnection backoff**
   - Force multiple connection failures, verify exponential backoff intervals (log capture or time measurement).

4. **Parallel workers during outage**
   - Spawn 20 goroutines calling `Dial()` simultaneously, then kill the SSH connection.
   - Verify: at most one worker returns error quickly, others block < timeout and eventually succeed after reconnection.

5. **Keepalive restart after reconnection**
   - Trigger reconnection, verify old keepalive goroutine stopped, new keepalive started with correct interval.

6. **Existing tests**
   - All current `ssh/tunnel` unit tests must pass unchanged.

## Rollout

- No configuration changes required.
- Deploy new binary; tunnels auto-reconnect on next failure.
- If running with older binary, no migration needed.
- No downtime or state transfer.
