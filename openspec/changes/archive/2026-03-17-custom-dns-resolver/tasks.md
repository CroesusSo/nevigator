## 1. Configuration Updates

- [x] 1.1 Add `CustomDNSServers []string` field to `EvaluatorConfig` in `pac/config.go`
- [x] 1.2 Add yaml tag `custom_dns_servers` to `EvaluatorConfig` field
- [x] 1.3 Add `CustomDNSServers []string` field to `GFW` struct in `cmd/proxy-pac/main.go`
- [x] 1.4 Add yaml tag `custom_dns_servers` to `GFW` field
- [x] 1.5 Ensure `loadConfig` sets empty slices (zero values) by default

## 2. PAC DNS Modifications

- [x] 2.1 Add `resolver *net.Resolver` field to `dnsCache` struct in `pac/dns.go`
- [x] 2.2 Change package function `lookupHost` to method `(c *dnsCache) lookupHost(ctx, host)`; inside, use `c.resolver` if non-nil else `net.DefaultResolver`
- [x] 2.3 Update `lookupHost` calls in `pac/builtins.go` to `cache.lookupHost(ctx, host)`
- [x] 2.4 In `pac/evaluator.go:NewEvaluator`, after creating `dnsCache`, if `cfg.CustomDNSServers` non-empty, create custom resolver and assign to `e.dnsCache.resolver`
- [x] 2.5 Create helper function `createCustomResolver(servers []string) *net.Resolver` (in `pac/dns.go` or `main.go`) with dial logic
- [x] 2.6 Add log in `NewEvaluator` when custom DNS is enabled: `[PAC] Using custom DNS: %v`

## 3. GFW Fetcher Modifications

- [x] 3.1 In `cmd/proxy-pac/main.go`, after `gfwFetcher := gfw.NewFetcher(...)`, check `cfg.GFW.CustomDNSServers`
- [x] 3.2 If non-empty, create custom `net.Dialer` with `Resolver: CreateCustomResolver(cfg.GFW.CustomDNSServers)`
- [x] 3.3 Create custom `http.Transport` with `DialContext: dialer.DialContext`
- [x] 3.4 Create custom `http.Client{Transport: transport, Timeout: httpTimeout}`
- [x] 3.5 Assign to `gfwFetcher.HTTPClient`
- [x] 3.6 Log: `[GFW] Using custom DNS: %v`

## 4. Verification

- [x] 4.1 Build binary successfully
- [x] 4.2 Run unit tests for PAC and GFW packages (should pass)
- [x] 4.3 Simulate DNS hijacking (e.g., `/etc/hosts` maps raw.githubusercontent.com to 0.0.0.0) and verify GFW fetch succeeds when custom DNS configured (manual test)
- [x] 4.4 Test PAC `dnsResolve` returns correct IP with custom DNS despite system DNS hijacking (manual test)
- [x] 4.5 Test empty `custom_dns_servers` uses system DNS (default behavior) (manual test)
- [x] 4.6 Verify log messages appear at startup (manual test)
