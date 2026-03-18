## 1. Core Implementation

- [x] 1.1 In `proxy/server.go`, within `NewServer`, refactor handler setup:
  - Create `mux` as before and register admin routes (via `RegisterRoutes`).
  - Create `topHandler` that intercepts `CONNECT` and dispatches to `s.handleHTTPConnect`.
  - Set `s.httpServer.Handler = topHandler` (instead of `mux`).
  - When creating `s.httpsServer`, also set its `Handler = topHandler`.
- [x] 1.2 Ensure `s.handleHTTPConnect` remains unchanged and continues to work with the wrapper.
- [x] 1.3 Fix `handleHTTPConnect` to extract target from `r.Host` (CONNECT target) instead of `r.URL.Path`.

## 2. Testing

- [x] 2.1 Add integration test (`TestConnectRouting`) that sends a CONNECT request to an HTTP and HTTPS listener and expects non-404 response (e.g., 200 or 502 on route failure).
- [x] 2.2 Run all existing tests to confirm no regressions.
- [ ] 2.3 Manually verify with `curl -x http://127.0.0.1:8080 https://example.com` and `curl -x https://127.0.0.1:8443 https://example.com`.
