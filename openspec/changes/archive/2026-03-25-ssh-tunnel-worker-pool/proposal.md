## Why

The current SSH tunnel implementation uses a single `sync.Mutex` to protect all state in the `Dial` method. Under heavy HTTPS request load with multiple concurrent connections through the same SSH tunnel, all `Dial` goroutines serialize on this mutex, causing observable lag and throughput degradation. Additionally, the reconnection logic has a bug where a tunnel can become permanently stuck in a non-connected state.

## What Changes

- Replace mutex-based `Tunnel` with a lock-free design using atomic state and a worker pool pattern
- Reduce lock contention from O(concurrent_dials) to O(1) in the hot path
- Fix reconnection bug: ensure the tunnel correctly transitions from error states and initiates reconnection
- Introduce backpressure via a buffered task channel; `Dial` calls that find the channel full receive `ErrOverload` immediately
- Add a separate failure signaling channel (`failCh`) for workers to non-blocking signal connection issues to the manager
- Manager goroutine owns the connection lifecycle; it listens on `failCh` and performs (re)connection with exponential backoff
- Atomic state variables (`client`, `state`, `closing`) replace mutex-protected fields
- No mutexes in the hot path: safe shutdown achieved via atomic `closing` check, non-blocking channel send, and context cancellation

The `Tunnel` struct fields become unexported and the internal concurrency model changes, but the public API (`Dial`, `Close`) remains identical. No breaking changes for users of the package.

## Capabilities

### Modified Capabilities
<!-- No existing capabilities have their REQUIREMENTS changed. This is an implementation-only improvement. -->

## Impact

- **Files modified**: `ssh/tunnel.go` (complete rewrite), `ssh/manager.go` (minor adjustments if needed), `proxy/server.go` (unchanged), `router/router.go` (unchanged)
- **Dependencies**: No new external dependencies; uses `sync/atomic` (already in use) and existing channels
- **Admin UI**: No changes; status endpoint continues to work
- **Configuration**: No new config options; but performance characteristics improve significantly under load
- **Testing**: Existing tests (`ssh/tunnel_test.go`) should continue to pass; may need minor adjustments for timing due to worker pool behavior
- **Metrics**: Could expose `taskCh` length and worker count in future, but not required
