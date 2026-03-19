# Implementation Tasks

## 1. Unify CONNECT Handler

- [x] In `proxy/server.go` `topHandler`, replace the conditional routing:
  - Remove the logic that checks `port == 443 && s.tlsConfig != nil` to decide between `handleHTTPConnect` and `handleHTTPSConnect`
  - Change to unconditionally call `s.handleCONNECT(w, r, host, port)` for all CONNECT requests
- [x] Rename `handleHTTPConnect` to `handleCONNECT` and remove any SNI-related protocol tracking code
- [x] In `handleCONNECT`, after hijacking, detect TLS for protocol tracking only:
  ```go
  protocol := "http"
  if _, isTLS := clientConn.(*tls.Conn); isTLS {
      protocol = "https"
  }
  ```
- [x] Remove the separate `handleHTTPSConnect` function entirely
- [x] Verify host extraction from CONNECT request works correctly (existing parsing is adequate)

## 2. Drop TLS_* Route Strategies

- [x] In `pac/config.go`: Remove `"TLS_TUN"`, `"TLS_SOCKS5"`, `"TLS_DIRECT"` from comments/valid values.
- [x] In `router/router.go`: Delete `case "TLS_TUN", "TLS_SOCKS5", "TLS_DIRECT":` from `dialWithRoute` switch. Ensure unknown strategies fall through to DIRECT.
- [x] In `router/router_test.go`: Remove tests for `TLS_TUN` and `TLS_SOCKS5` (e.g., `TestRouter_TLSRoute_*`).

## 3. Remove tlsSNI Builtin

- [x] In `pac/builtins.go`: Delete the entire `vm.Set("tlsSNI", ...)` registration.
- [x] Search repository for any remaining references to `tlsSNI` and remove if found.

## 4. Clean Up Admin UI (Optional)

- [x] In `admin/handlers.go`, verify `matchTypeOptions` does **not** include `"client_sni"`; remove if present to prevent creating unsupported SNI-based rules

## 5. Update Callers (if needed)

- [x] Verify that all calls to `evaluator.Evaluate` pass only `host`. The topHandler already does this. No other changes expected.

## 6. Tests

- [x] Add integration test: HTTPS listener (with TLS) handles CONNECT to a non-443 destination (e.g., 8443) successfully
- [x] Verify existing HTTPS listener tests still pass (listener startup, disabled without TLS)
- [x] Ensure all existing HTTP CONNECT tests pass (including IPv6 tests)
- [x] Test that HTTPS proxy to port 443 destinations still works (covered by existing tests but verify)
- [x] Run `go test ./...` and fix any failures

## 7. Documentation

- [x] Update `README.md` to remove references to TLS_* strategies and SNI-based routing if present.
- [x] Update any configuration examples that included TLS_* values.

