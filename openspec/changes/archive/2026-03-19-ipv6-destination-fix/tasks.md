# IPv6 Destination Handling Fix – Tasks

## Implementation
- [x] Add `formatDest(host string, port int) string` helper to `proxy/server.go`.
- [x] Update `handleHTTPConnect` to use `formatDest` for `dest`.
- [x] Update `handleHTTPSConnect` to use `formatDest` for `dest` (if applicable).
- [x] Update `handleSOCKS5` to use `formatDest` for `dest`.

## Testing
- [x] Write unit tests for `formatDest`:
  - IPv6 without brackets → add brackets
  - IPv6 with brackets already → no change
  - IPv4 → no change
  - Domain name → no change
- [x] Add integration test for HTTP CONNECT to IPv6 destination.
- [x] Add integration test for SOCKS5 CONNECT to IPv6 destination through direct and upstream proxy.
- [x] Run full test suite to ensure no regressions.

## Documentation & Release
- [x] Update README (IPv6 support note).
- [x] Add release note: "Fix IPv6 destination handling for CONNECT and SOCKS5."
- [x] Verify existing IPv4 and domain tests still pass.

## Review & Archive
- [x] Code review
- [x] QA smoke test on IPv6 network (optional)
- [x] Archive change after merge
