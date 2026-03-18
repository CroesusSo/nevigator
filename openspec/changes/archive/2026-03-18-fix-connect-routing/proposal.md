## Why

CONNECT requests (used for HTTPS through the proxy) currently return 404 because the HTTP mux is registered with pattern `/`, which only matches URL paths that start with `/`. CONNECT requests have a URL path like `www.google.com:443` (no leading slash), so they never reach the proxy handler. This breaks HTTPS proxying for both HTTP and HTTPS listeners.

## What Changes

- Replace the mux direct registration with a top-level handler that intercepts `CONNECT` requests and forwards them to `handleHTTPConnect`, while passing all other methods to the admin/API mux.
- This change will affect both the HTTP and HTTPS servers, ensuring CONNECT works on both ports.
- No changes to SOCKS5 (unaffected).

## Capabilities

### New Capabilities
- `connect-routing`: Defines how CONNECT requests are dispatched by the proxy server, ensuring they are handled correctly regardless of URL.Path.

### Modified Capabilities
*(none)*

## Impact

- Files: `proxy/server.go` (NewServer)
- Admin UI and API remain unaffected.
- SOCKS5 unchanged.
- All existing tests should continue to pass; new tests needed for CONNECT path through the server.
