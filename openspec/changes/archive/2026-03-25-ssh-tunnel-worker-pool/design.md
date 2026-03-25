## Context

The current `ssh.Tunnel` implementation (see `ssh/tunnel.go`) uses a `sync.Mutex` to protect all state access in the `Dial` method. Under high HTTPS request concurrency, when many goroutines attempt to dial through the same SSH tunnel simultaneously, they all contend for this single mutex. The critical section is small (checking `state` and `client`), but the serialization creates a bottleneck: all goroutines must queue, causing measurable lag. Additionally, the reconnection logic has a subtle bug: when `client.Dial` fails while the tunnel is in `stateConnected`, the code sets `state = stateReconnecting` but does not restart `connectLoop` (which only runs when `state == stateClosed`). This can leave the tunnel permanently unavailable until manually closed and reopened.

### Current Code Structure

```go
type Tunnel struct {
    mu            sync.Mutex
    state         string
    client        *ssh.Client
    closing       bool
    connectedChan chan struct{}
    // ...
}

func (t *Tunnel) Dial(dest string, timeout time.Duration) (net.Conn, error) {
    for {
        t.mu.Lock()
        // Check state, possibly start connectLoop
        t.mu.Unlock()
        // Wait on connectedChan or timeout
        // Then client.Dial(dest)
    }
}
```

### Constraints

- Must preserve the public API: `Tunnel.Dial` and `Tunnel.Close`
- Must maintain reconnection with exponential backoff
- Must handle graceful shutdown without panics
- Should provide backpressure when overloaded (reject rather than unbounded queue)
- No change to external behavior: SSH tunnel routing semantics remain identical
- Go 1.21 (has `atomic.Pointer`)

---

## Goals / Non-Goals

**Goals:**
- Eliminate mutex contention in the hot path of `Dial`
- Reduce per-dial latency under high concurrency
- Fix reconnection bug (tunnel can get stuck in `stateReconnecting`)
- Maintain API compatibility (no breaking changes for callers)
- Provide natural backpressure via bounded queues
- Keep manager-driven reconnection (single owner of connection lifecycle)
- Ensure safe shutdown without race conditions or panics

**Non-Goals:**
- Implement connection pooling (SSH channels are already multiplexed)
- Add metrics/exposition (can be done separately)
- Support TCP stream multiplexing within a single SSH channel (out of scope)
- Change reconnection policy (backoff remains exponential with jitter)
- Support multiple concurrent reconnection attempts (only one at a time)

---

## Decisions

### 1. Adopt Worker Pool Pattern

**Decision**: Use a fixed pool of worker goroutines that consume dial tasks from a buffered channel (`dialCh`). The `Dial` method enqueues tasks non-blocking; if the channel is full, it returns `ErrOverload` immediately.

**Rationale**:
- Eliminates mutex in hot path: workers read `client` atomically
- Natural concurrency limit via channel buffer size
- Backpressure to callers (no unbounded queueing)
- Fair FIFO ordering
- Standard Go idiom (worker pool)

**Alternatives Considered**:
- **Single manager goroutine (serial)**: Simpler but becomes bottleneck if `client.Dial` is slow
- **Atomic-only (no workers)**: Would require each `Dial` to read atomics directly; still would need to coordinate reconnection. Worker pool cleanly separates concerns.

### 2. Atomic State with `atomic.Pointer` and `atomic.Value`

**Decision**: Replace mutex-protected fields with atomically accessible state:
- `client atomic.Pointer[ssh.Client]`
- `state atomic.Value` (string)
- `closing atomic.Bool`

**Rationale**:
- Workers need only read `client` — atomic load has no blocking
- State updates (connect, close) happen in single goroutine (manager) → no write contention
- Simple and safe: one writer, many readers
- `atomic.Pointer` available in Go 1.19+

**Alternatives Considered**:
- `sync/atomic.Value` for `client` (same as `atomic.Pointer`)
- `RWMutex` for all state (still a lock, albeit shared)

### 3. Dedicated Manager Goroutine + Connection Loop

**Decision**: A single manager goroutine owns the connection lifecycle. It runs `connect()` initially and then listens for `failCh` signals from workers to trigger reconnection. The manager does **not** directly process dial tasks; workers consume `dialCh`.

**Rationale**:
- Clear separation: manager owns connection, workers use it
- Reconnection decisions centralized → no concurrent reconnect attempts
- Failures from any worker can signal manager (non-blocking to avoid worker slowdown)
- Simple logic: manager only responds to `failCh` and `shutdown`

**Alternatives Considered**:
- Manager also dispatches tasks (extra complexity, no benefit)
- Workers directly call `connect()` when they detect failure (risks concurrent reconnect storms)

### 4. Non-Blocking Failure Signaling (`failCh`)

**Decision**: Workers that experience a `client.Dial` error send a signal on a buffered `failCh` using a non-blocking `select { case failCh <- struct{}{}: default: }`.

**Rationale**:
- Manager must know connection is bad, but we don't want to flood it
- Non-blocking send means fast failure path (worker doesn't pause)
- Buffered channel absorbs bursts (size 10 sufficient)
- Manager will reconnect once; subsequent failures during reconnect are ignored

**Alternatives Considered**:
- Unbuffered `failCh`: workers would block until manager reads → unacceptable (slows failure path)
- Dropping all signals: some failures might be missed if manager is in backoff

### 5. Non-Blocking Enqueue Without Mutex

**Decision**: Do *not* use a mutex for enqueue. Instead, make workers listen on `ctx.Done()` in addition to `dialCh` (using `select`), and never close `dialCh`. `Dial` simply checks `closing` atomically, then non-blocking sends to `dialCh`. If channel is full, returns `ErrOverload`. `Close` cancels context; workers exit gracefully even with pending tasks.

**Rationale**:
- Eliminates the last lock from the hot path entirely
- Simpler: no need to coordinate `dialCh` closure with enqueue
- Safe: workers exit via context; pending tasks may be left in `dialCh` but that's fine (GC'd when tunnel freed)
- `Dial` can still be called during shutdown but will see `closing=true` after the atomic check

**Why This Works**:
The original concern was: `Dial` checks `closing=false`, then `Close` closes `dialCh` before the send → panic. If we never close `dialCh`, that panic cannot happen. Workers unblock via `ctx.Done()`, not channel close.

**Alternatives Considered**:
- Keep `enqueueMu`: adds a tiny lock in hot path, unnecessary
- Close `dialCh` in `Close`: requires mutex or risk of panic, rejected

### 6. Shutdown via Context + WaitGroup

**Decision**: Pass a `context.Context` (canceled by `Close`) to all goroutines. Workers and manager select on `ctx.Done()` to exit. `Close` calls `wg.Wait()` to ensure clean exit before returning.

**Rationale**:
- Standard graceful shutdown pattern in Go
- Guarantees all goroutines exit before `Close` returns
- No need to close `dialCh` (channel close is for *senders*, not receivers); instead workers exit when context done and drain `dialCh` may be left with stale tasks (acceptable)

---

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Manager becomes bottleneck if connection establishment is slow** | Reconnection delay while workers wait | `dialSSH` already has 30s timeout; could reduce to 5-10s. Manager only blocks during `dialSSH` (not during backoff sleep if we select with context). |
| **Worker pool size too small → queue buildup, `ErrOverload`** | Increased rejections under load | Document recommended worker count (e.g., `2 * runtime.GOMAXPROCS()` or configurable). Default to 8 (sufficient for most cases). |
| **Worker uses `client` after manager closes it** | Error returned to user, extra reconnection signal | Harmless: `client.Dial` on closed SSH client returns error. Worker sends error, which may trigger extra reconnection (idempotent). |
| **`taskCh` buffer too large → unbounded memory growth** | OOM if callers ignore backpressure and keep enqueuing | `Dial` uses non-blocking send; never blocks, so buffer never accumulates more than its capacity. Caller receives `ErrOverload` quickly. |
| **In-flight tasks after `Close` still processed** | Unnecessary work, but harmless | Workers check `ctx.Err()` before processing tasks; if closing, they return `ErrTunnelClosed`. Minimizes post-close processing. |
| **Lost `failCh` signals during burst failures** | Manager might miss some failures, but at least one will get through | Buffer size 10 + non-blocking send from many workers: some will succeed. Manager will reconnect on first signal. Missing subsequent signals is fine. |
| **`client.Dial` hangs longer than caller's timeout** | Worker blocked, reduces effective worker pool size | `client.Dial` uses TCP with no separate timeout. Could wrap with `context.WithTimeout` if needed, but SSH library should respect net dial timeouts. |
| **Reconnection loop blocks indefinitely on permanent error** | Manager stuck in `dialSSH` retry if `isPermanentError` logic broken | Ensure `isPermanentError` correctly identifies permanent failures; if uncertain, limit retry attempts (already: immediate return on permanent). |
| **`dialSSH` uses hard-coded 30s timeout** | Slow failure detection during manager reconnect | Could make configurable or reduce to 10s. Not critical; this is internal to manager and doesn't block workers. |

---

## Migration Plan

### Phase 1: Implementation
1. Implement new `Tunnel` struct with channels and atomic state
2. Copy existing public methods: `Dial`, `Close`
3. Add internal methods: `manager()`, `connect()`, `worker()`
4. Preserve same error values (`ErrTunnelClosed`, etc.)
5. Update `ssh/manager.go` if necessary (likely no changes; it just calls `Dial`)

### Phase 2: Testing
1. Run existing unit tests (`ssh/tunnel_test.go`) — they should pass unchanged
2. Add new benchmark: concurrent `Dial` calls with successful connection (measure latency tail)
3. Add test for `ErrOverload` when `dialCh` full
4. Add test for reconnection bug fix: simulate failure after connected, verify subsequent `Dial` eventually succeeds

### Phase 3: Rollout
- Binary upgrade: replace old binary with new one
- No config changes needed
- No data migration needed
- Admin UI unchanged

### Rollback
- Revert to previous binary
- No state migration required; new `Tunnel` is completely self-contained
- Existing connections in flight will be dropped on shutdown, but that's normal

---

## Open Questions

1. **Recommended worker pool size**: The default should be **8 workers** (not 20). This is sufficient for typical SSH tunnel use cases where each `client.Dial` involves network I/O. The worker count can be made configurable in a future change if needed.

2. **`dialCh` buffer size**: What is appropriate? If buffer=100 and workers=20, that's 5 levels of queuing. That seems reasonable for burst traffic. Should be configurable? Likely keep fixed for now (100) and document.

3. **Should workers continue after `client.Dial` error?** Yes, they should keep processing tasks; the failure is per-dial, not fatal.

4. **Should we add metrics?** Not required for this change, but could add later: counter for `dialCh` full events, worker utilization, etc.

5. **Should `failCh` be unbuffered?** We buffered 10; is that enough? Given we coalesce signals, 1 would be sufficient but small buffer allows bursts. 10 is fine.

6. **Dial timeout semantics**: Currently `Dial`'s timeout includes time waiting in `taskCh`. That remains true. Should we expose the wait separately? Not needed.
