## Context

The `Tunnel.Dial` method in `ssh/tunnel.go` has a critical concurrency bug:

```go
// Current buggy pattern (simplified)
t.mu.Lock()
defer t.mu.Unlock()  // Main goroutine will unlock
go func() {
    t.cond.Wait()    // Wait unlocks mutex and transfers ownership to this goroutine
    close(waitDone)
}()
// ...
return // defer Unlock() runs → panic: unlock of unlocked mutex
```

`sync.Cond.Wait()` atomically unlocks the mutex and blocks. Ownership of the mutex is transferred to the waiting goroutine, which will re-lock before `Wait()` returns. The original goroutine must **not** subsequently call `Unlock()` – but the current code does because of the `defer`.

The bug triggers reliably when a timeout occurs while the `Wait` goroutine is still blocked.

Current workaround: none – the application crashes.

## Goals / Non-Goals

**Goals:**
- Eliminate the double-unlock panic completely
- Preserve all existing behavioral semantics:
  - `Dial` blocks until tunnel is connected or timeout expires
  - Timeout bounds total time including tunnel establishment
  - Upon connection failure, retry with exponential backoff
  - Permanent errors (auth failures, etc.) stop retrying
  - Multiple concurrent `Dial` calls synchronize correctly
- Keep code simple, idiomatic Go, and easy to maintain

**Non-Goals:**
- Change the SSH tunnel API or `Tunnel` interface
- Modify timeout, backoff, or retry semantics
- Add new configuration options
- Redesign the tunnel manager or router integration

## Decisions

### Decision 1: Replace `sync.Cond` with channel-based broadcast

**Approach**: Use a `chan struct{}` field (`connectedChan`) in `Tunnel`. When the tunnel becomes connected (or changes state), close the current channel to broadcast to all waiters. Waiters select on the channel with a timeout.

**Rationale**:
- Channels are the idiomatic Go mechanism for goroutine communication
- Simpler mental model: no mutex ownership transfer, no need to coordinate between goroutines
- `close(ch)` broadcasts to **all** pending receivers simultaneously (like `cond.Broadcast()`)
- No risk of forgetting to unlock or double-unlock – each goroutine manages its own blocking

**Alternatives considered**:
- Keep `sync.Cond` but restructure code so `Wait` is called in the same goroutine that unlocks. This would require substantial state machine changes and doesn't improve readability.
- Use a `sync.WaitGroup` – not suitable because we need repeated notifications (multiple connection cycles).

### Decision 2: Store `connectedChan` in `Tunnel` struct, recreate on each connection cycle

**Pattern**:
```go
type Tunnel struct {
    mu            sync.Mutex
    connectedChan chan struct{} // closed to signal connected state
    // ...
}

// In connectLoop, after successful connection:
t.mu.Lock()
t.connectedChan = make(chan struct{}) // prepare for next cycle
t.client = client
t.state = stateConnected
t.mu.Unlock()
close(oldChan) // broadcast to waiters
```

**Rationale**:
- Recreating the channel after each connection ensures future waiters don't receive stale notifications from the previous connection event
- The old channel lives as long as any goroutine is still blocked on it; closing it wakes them all
- Under the mutex, we atomically replace the channel so new waiters will use the fresh one

### Decision 3: `Dial` loop structure

```go
for {
    t.mu.Lock()
    // Check immediate conditions (connected, permanent error)
    if t.client != nil && t.state == stateConnected {
        t.mu.Unlock()
        return t.client.Dial("tcp", dest)
    }
    if isPermanentError(t.lastErr) {
        t.mu.Unlock()
        return nil, t.lastErr
    }
    remaining := time.Until(deadline)
    if remaining <= 0 {
        t.mu.Unlock()
        return nil, fmt.Errorf("dial timeout: %w", t.lastErr)
    }
    // Copy current notifier under lock, then unlock
    notifier := t.connectedChan
    t.mu.Unlock()

    select {
    case <-notifier:
        continue // state changed, re-check
    case <-time.After(remaining):
        // Final check under lock
        t.mu.Lock()
        if t.client != nil && t.state == stateConnected {
            t.mu.Unlock()
            return t.client.Dial("tcp", dest)
        }
        t.mu.Unlock()
        return nil, fmt.Errorf("dial timeout")
    }
}
```

**Rationale**:
- All mutex operations are confined to the same goroutine – no cross-goroutine ownership
- The `notifier` channel is captured under the mutex; it's safe because closing the old channel only happens after a new one is installed
- Timeout handling occurs in the same goroutine, avoiding race conditions
- The pattern naturally supports multiple concurrent `Dial` callers – they all wait on the same notifier

## Risks / Trade-offs

**Risk**: Channel close may race with channel creation, leaving a waiter blocked forever if the notifier is replaced between capture and `select`.
- **Mitigation**: The notifier is replaced under `t.mu.Lock()`. All waiters capture the notifier while holding the lock. The old notifier is closed **after** installing the new one, within the same critical section. Any goroutine that captured the old notifier will either still be blocked (and will be woken by the close) or has already returned.

**Risk**: Leak of the old channel if no goroutine is waiting on it when closed.
- **Mitigation**: Acceptable – the channel will be garbage collected once all references drop. The number of channels allocation is bounded by connection attempts, not by concurrent waiters.

**Trade-off**: Slightly more memory allocations (one channel per connection cycle vs. one `sync.Cond`). This is negligible compared to the complexity reduction and safety gain.

**Risk**: Existing tests may not cover the timeout path adequately, so regression possible.
- **Mitigation**: Add targeted tests that simulate slow connections and timeouts to exercise the fixed code path.

## Migration Plan

1. Implement the changes in `ssh/tunnel.go`:
   - Add `connectedChan` field to `Tunnel`
   - Initialize `connectedChan` in `NewTunnel` (create initial `make(chan struct{})`)
   - Rewrite `Dial` loop as per Decision 3
   - Update `connectLoop` to reinitialize `connectedChan` after successful connection
   - Also update on permanent failure? No – on permanent failure the tunnel is closed; waiters will get permanent error. But we should still close the channel to wake them? Actually permanent failure returns an error and `Dial` will read `t.lastErr` directly; waiters don't need to be woken. However if they're waiting while `connectLoop` is trying and hits a permanent error, they'll wait until timeout. That's acceptable – they'll timeout with the appropriate error (t.lastErr may be permanent but they check it after waking). So channel close only on successful connection is sufficient.

2. Remove all `sync.Cond` usage (`t.cond` field can be removed)
3. Update `GetTunnelsStatus` (manager.go) if it references `t.cond` (it doesn't – only locks `t.mu`)
4. Run existing tests; ensure no regressions
5. Add new tests to simulate timeout during connection (e.g., with a mock `dialSSH` that sleeps longer than timeout)
6. Manual smoke test: configure an SSH proxy with an unreachable address and verify the proxy-pac binary doesn't panic

No database migrations, config changes, or downtime required. The change is backward-compatible at the API level.

## Open Questions

None identified. The design is straightforward and addresses the root cause directly.