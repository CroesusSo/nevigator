# IPv6 Destination Handling Fix – Proposal

## Problem

The proxy supports IPv6 in many places (SOCKS5 address types, listening addresses), but fails when the **destination** (the target of CONNECT or SOCKS5 connection) is an IPv6 literal.

- HTTP CONNECT: `r.Host` like `[::1]:443` is split and then reassembled as `::1:443` (missing brackets). `net.Dial("tcp", dest)` rejects this.
- SOCKS5 upstream: similar issue when constructing `dest` for `Route` and when sending the CONNECT request to the upstream proxy.
- TUN/DIRECT routes to IPv6 destinations also fail.

The core issue is that `net.SplitHostPort` returns the host without brackets, and later usage assumes IPv4 or domain names. For IPv6, brackets are required when recombining into a `host:port` string for `net.Dial` and SOCKS5 request building.

## Proposed Solution

Introduce a helper `formatDest(host string, port int) string` that wraps IPv6 literals in brackets.

- In `handleHTTPConnect`, `handleHTTPSConnect`, `handleSOCKS5`: replace `fmt.Sprintf("%s:%d", host, port)` with `formatDest(host, port)`.
- Ensure any other `dest` construction follows the same pattern.

No changes to the native rule engine or PAC evaluation are required—the host passed to `Evaluator.Evaluate` remains unchanged (already correct).

## Benefits

- Full IPv6 support for destination addresses.
- Consistent handling across HTTP CONNECT, SOCKS5 client, and TUN/DIRECT routing.
- Minimal code change, low risk.

## Scope

- Modify `proxy/server.go`:
  - Add `formatDest` helper.
  - Update `handleHTTPConnect`, `handleHTTPSConnect` (if needed), `handleSOCKS5`.
- Add unit tests for `formatDest` and integration tests for IPv6 CONNECT.
- Update documentation (README) to note IPv6 destination support.

## Out of Scope

- IPv6 in PAC scripts (already works via `dnsDomainIs`, etc.).
- IPv6 upstream proxy address (already works with bracketed format).
- NAT64/DNS64 considerations – out of scope.

## Risks & Mitigations

- **Risk:** Forgetting to bracket in some code path.  
  Mitigation: Centralize via `formatDest`; add tests for coverage.
- **Risk:** Breaking existing IPv4 behavior.  
  Mitigation: `formatDest` leaves IPv4 and domain names unchanged; tests cover both.

---

After this change, the proxy will correctly route CONNECT requests to IPv6 destinations and support IPv6 through SOCKS5 upstreams end-to-end.