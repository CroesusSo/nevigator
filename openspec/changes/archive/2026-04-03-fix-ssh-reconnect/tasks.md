## 1. Tunnel Manager Adjustments

- [x] 1.1 Modify `Tunnel.manager()` to always call `connect()` on `failCh` signals (remove `isConnected()` check)
- [x] 1.2 Modify `Tunnel.connect()` to allow reconnection from `stateConnected` (remove `stateConnected` from guard, keep only `stateConnecting`)

## 2. Failure Detection & Marking Broken

- [x] 2.1 Modify `Tunnel.keepaliveLoop()` to set `client = nil` via atomic swap, close old client, and signal `failCh` when keepalive failures exceed threshold
- [x] 2.2 Modify `Tunnel.worker()` to set `client = nil` via atomic swap and close old client when `client.Dial()` returns an error (before signaling `failCh`)
- [x] 2.3 Verify workers already wait when `client == nil` (existing condition variable logic)

## 3. Testing

- [x] 3.1 Add unit test: simulate keepalive-triggered reconnection, verify `client` becomes nil, `failCh` signaled, manager reconnects, new client established, keepalive restarted
- [x] 3.2 Add unit test: simulate worker-detected failure, verify first call returns error, subsequent calls block and succeed after reconnection
- [x] 3.3 Add unit test: verify exponential backoff increases after repeated failures
- [x] 3.4 Add unit test: verify parallel Dial calls during outage produce only one reconnection attempt and all succeed after recovery
- [x] 3.5 Run existing SSH tunnel tests and confirm all pass

## 4. Verification & Documentation

- [x] 4.1 Build binary and run full test suite (`make test`)
- [x] 4.2 Update any inline comments to reflect new reconnection behavior
- [x] 4.3 Ensure no new data races (`go test -race ./ssh`)
- [x] 4.4 Review logs to confirm reconnection messages appear as expected
