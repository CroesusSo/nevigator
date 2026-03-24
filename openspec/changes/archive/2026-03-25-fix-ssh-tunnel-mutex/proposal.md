## Why

The SSH tunnel implementation has a critical concurrency bug causing crashes with the panic: "fatal error: sync: unlock of unlocked mutex". The root cause is the misuse of `sync.Cond` in `Tunnel.Dial` – specifically, calling `Wait()` in a separate goroutine while the main goroutine retains responsibility for unlocking the mutex. When a timeout occurs, the main goroutine attempts to `Unlock()` a mutex that was transferred to the waiting goroutine, triggering a panic.

This bug makes the SSH tunnel feature unstable and prone to crashes during connection timeouts or slow networks.

## What Changes

- **Implementation Fix**: Replace `sync.Cond` with a channel-based notification pattern that ensures mutex ownership stays within the same goroutine that acquired it
- **Refactor `Tunnel.Dial`**: Remove the incorrect goroutine + `Wait()` pattern; use a `connectedChan` for broadcast notifications with proper timeout handling
- **Update `Tunnel.connectLoop`**: Close `connectedChan` when a connection is established to wake up all waiters
- **Preserve behavior**: The fix maintains all existing semantics – timeout handling, retry logic, reconnection backoff, and permanent error detection remain unchanged

## Capabilities

This is an implementation-level bug fix that tightens the SSH tunneling requirements to ensure correctness under concurrency.

- **Modified Capabilities**: `ssh-tunnel-proxy` – adding explicit requirement that the tunnel SHALL be free of data races and shall never panic due to mutex misuse, even under timeout, concurrent access, and connection failures.

## Impact

- **Files modified**: `ssh/tunnel.go` (primary). `ssh/manager.go` may be inspected but requires no changes.
- **No API/Config changes**: The fix is internal; no changes to configuration schema, command-line flags, or public interfaces.
- **Risk**: Low – change is localized, eliminates a crash bug, and simplifies the concurrency pattern.
- **Testing**: Existing tests must pass; new unit tests will cover timeout and concurrent-Dial edge cases. Integration tests recommended.