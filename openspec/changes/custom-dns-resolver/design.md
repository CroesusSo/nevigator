## Context

The system has two critical components that perform DNS lookups:
1. **PAC evaluator**: Uses `net.DefaultResolver` in `pac/dns.go:lookupHost()` for `dnsResolve()` and `isResolvable()` builtins
2. **GFW fetcher**: Uses `http.Client` with default transport, which uses `net.DefaultResolver` for hostname resolution

In GFW-censored environments, system DNS (configured in `/etc/resolv.conf`) is often hijacked to return `0.0.0.0` for blocked domains. This causes:
- GFW fetch failures: connects to `0.0.0.0:443` → connection refused
- PAC routing failures: `dnsResolve("example.com")` returns wrong IP

The solution is to allow configuring a custom DNS resolver that bypasses system DNS and queries trusted public DNS servers directly.

## Goals / Non-Goals

**Goals:**
- Provide configurable custom DNS for both PAC evaluator and GFW fetcher
- Use Go's `net.Resolver` with a custom `Dial` function to send DNS queries to specified servers
- Maintain backward compatibility: empty server list uses system DNS
- Keep changes localized and testable
- Add clear logging indicating which DNS mode is active

**Non-Goals:**
- Implement DNS-over-HTTPS (DoH) – out of scope, adds complexity
- Dynamic DNS server health checking – configuration is static
- Fallback from custom DNS to system DNS on failure – fail fast, let user adjust config
- Change DNS behavior for other parts of the system (router, proxy) – only PAC and GFW

## Decisions

### Custom Resolver via net.Resolver.Dial
**Decision:** Use `net.Resolver{PreferGo: true, Dial: customDial}` where `customDial` dials each configured DNS server (UDP) sequentially until one succeeds.

**Rationale:**
- `net.Resolver` handles DNS protocol details, timeouts, retries, TCP fallback
- `Dial` override allows selecting specific DNS servers, bypassing system config
- Pure stdlib, no external dependencies
- Works for both PAC (direct calls) and GFW (via `http.Transport.Dialer`)

**Alternatives:**
1. Manual DNS client with `net.Dial` + raw DNS packets – rejected: reinventing the wheel, error-prone
2. Third-party DoH library – rejected: adds dependencies, overkill
3. Hardcode IPs for specific domains – rejected: brittle, GitHub CDN changes often

### Configuration Placement
**Decision:** Add `custom_dns_servers` to BOTH `pac:` and `gfw:` config sections. They can be configured independently, but users will likely set them identically.

**Rationale:**
- Each component has its own config struct (`EvaluatorConfig` and main's `GFW` struct)
- Avoids creating a shared top-level `dns:` section that would need plumbing through multiple layers
- Flexibility to disable custom DNS for one component while keeping it for the other if needed
- Simple YAML extension: just add a list field

**Alternatives:**
1. Top-level `dns:` section – rejected: requires changing config loading logic more broadly
2. Global singleton resolver – unnecessary abstraction; let each component create its own

### PAC Integration: dnsCache Holds Resolver
**Decision:** Extend `dnsCache` struct with `resolver *net.Resolver` field. Initialize it in `NewEvaluator` if `cfg.CustomDNSServers` is non-empty. Change `lookupHost` from package function to method `(c *dnsCache) lookupHost(ctx, host)`; inside, use `c.resolver` if set, else `net.DefaultResolver`.

**Rationale:**
- `dnsCache` already exists and is passed to `registerBuiltins`, so attaching resolver here is natural
- Makes `lookupHost` testable and avoids global variables
- Minimal changes to existing code: only need to change `cache.lookupHost` calls in closures

**Alternatives:**
1. Package-level `var customResolver *net.Resolver` – rejected: global state, harder to test
2. Make `lookupHost` a closure capturing resolver – rejected: would need to change `registerBuiltins` signature; less clean

### GFW Integration: Set HTTPClient Externally
**Decision:** In `cmd/proxy-pac/main.go`, after creating `gfwFetcher`, if `cfg.GFW.CustomDNSServers` is non-empty, construct a custom `http.Client` with `Transport.DialContext` using a `net.Dialer` that has `Resolver` set to the custom resolver. Then assign `fetcher.HTTPClient = customClient`.

**Rationale:**
- `gfw.NewFetcher` creates a default client; we can override it after creation
- No need to modify `fetcher.go` itself; keeps component simple
- The fetcher already exposes `HTTPClient` as a field (not private), so it's intended to be settable

**Alternatives:**
1. Add `WithCustomDNS` option to `NewFetcher` – rejected: would require changing `fetcher.go` and adding more complexity
2. Create a separate `FetcherWithDNS` constructor – rejected: diverging APIs, unnecessary

### Logging
**Decision:** Log at INFO level when custom DNS is enabled for PAC and GFW:
- PAC: `log.Printf("[PAC] Using custom DNS: %v", cfg.CustomDNSServers)`
- GFW: `log.Printf("[GFW] Using custom DNS: %v", cfg.GFW.CustomDNSServers)`

Also log when using system DNS (debug level? probably not needed). Log at startup to be visible in logs.

## Risks / Trade-offs

[Custom DNS servers themselves may be blocked in some networks] → Mitigation: configurable empty list to disable; user can choose alternative servers.
[Custom DNS queries use UDP 53, may be rate-limited or blocked] → Mitigation: Use well-known reliable servers; if both UDP/TCP 53 blocked, no DNS works; out of scope for this change.
[Increased latency vs local DNS cache] → Acceptable: fetches are infrequent; PAC lookups are cached in `dnsCache`.
[No fallback to system DNS if custom DNS fails] → Acceptable: fail fast and alert user to adjust config; complexity of fallback not worth it.

## Migration Plan

1. Add config fields to `EvaluatorConfig` and `GFW` struct with yaml tags
2. Add defaults in `loadConfig` (empty slices by default, meaning system DNS)
3. Modify `pac/dns.go`: add `resolver *net.Resolver` to `dnsCache`, change `lookupHost` to method
4. Modify `pac/evaluator.go`: in `NewEvaluator`, if `cfg.CustomDNSServers != nil`, create resolver and assign to `e.dnsCache.resolver`
5. Modify `pac/builtins.go`: change `lookupHost(ctx, host)` to `cache.lookupHost(ctx, host)`
6. Modify `cmd/proxy-pac/main.go`: for GFW, after `NewFetcher`, if `cfg.GFW.CustomDNSServers != nil`, build custom HTTP client and set `fetcher.HTTPClient`
7. Add logging in both initialization paths
8. Update default config YAML generation (if any) to include commented examples
9. Test with DNS hijacking simulated (e.g., `/etc/hosts` override or local DNS returning 0.0.0.0)

Rollback: Disable custom DNS by setting `custom_dns_servers: []` or empty in config; revert binary.

## Open Questions

- Should we allow specifying DNS servers per-component (PAC vs GFW) differently? Yes, configurable independently.
- Should we include a timeout for DNS queries separate from global timeouts? The `net.Resolver` uses the context's timeout; PAC already has `dns_timeout`, GFW fetch timeout can be reused.
- Should we skip IPv6? We filter to IPv4 anyway in `lookupHost`, but the resolver may still return IPv6; we filter. That's fine.
- Should we validate DNS server addresses in config (must be `host:port`)? Probably not strictly necessary; `net.Dial` will fail if invalid; log error on startup.
