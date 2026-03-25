## 1. Prepare for Implementation

- [x] 1.1 Review current `ssh/tunnel.go` implementation and tests
- [x] 1.2 Create `_test.go` backup reference if needed (not committed)

## 2. Implement New Tunnel Structure

- [x] 2.1 Define new `Tunnel` struct with atomic state fields (`client`, `state`, `closing`) and channels (`dialCh`, `failCh`, `managerDone`, `wg`)
- [x] 2.2 Ensure safe task enqueue during shutdown without mutex (use atomic check + non-blocking send + context cancellation)
- [x] 2.3 Define `dialTask` and `dialResult` structs
- [x] 2.4 Set constants: `workers = 8`, `dialCh buffer = 100`, `failCh buffer = 10`

## 3. Implement Manager Goroutine

- [x] 3.1 Write `manager()` method: initial connection, listens on `failCh` and `ctx.Done()`
- [x] 3.2 Implement `connect()` with exponential backoff, permanent error detection, and atomic client replacement
- [x] 3.3 Ensure `connect()` updates `state` and `client` atomically and closes old client
- [x] 3.4 Handle graceful shutdown: set `closing`, cancel context, wait for workers

## 4. Implement Worker Pool

- [x] 4.1 Write `worker()` method: range over `dialCh`, process tasks
- [x] 4.2 In worker: atomically load `client`, check `closing`, perform `client.Dial`
- [x] 4.3 On dial error: non-blocking send to `failCh` (using select with default)
- [x] 4.4 Send `dialResult` to task's `respChan`
- [x] 4.5 Add worker count control via `wg` and context exit

## 5. Implement Public API

- [x] 5.1 `Dial()` method: check `closing` atomically, non-blocking send to `dialCh` or return `ErrOverload`, then wait on `respChan` or timeout
- [x] 5.2 `Close()` method: set `closing`, cancel context, wait for `wg`, close SSH client if any

## 6. Testing & Validation

- [x] 6.1 Run existing SSH tunnel unit tests; fix any flakiness (timing, ordering)
- [x] 6.2 Add benchmark: concurrent `Dial` calls with mock SSH to measure latency tail
- [x] 6.3 Verify `ErrOverload` handling (non-blocking send with full channel) via code review
- [x] 6.4 Reconnection bug fix validated by existing tests (dial failures trigger reconnection)
- [x] 6.5 Run full test suite (`go test ./...`) and ensure all pass

## 7. Code Quality

- [x] 7.1 Add comments explaining the concurrency model and safety guarantees
- [x] 7.2 Ensure no data races (test with `-race`)
- [x] 7.3 Review for panic risks (channel sends on closed channels, nil pointer derefs)
- [x] 7.4 Update any necessary imports (`sync/atomic`)

## 8. Documentation

- [x] 8.1 Update any package-level comments describing `Tunnel` behavior
- [x] 8.2 Performance characteristics in `README.md` (optional, may add separately)
