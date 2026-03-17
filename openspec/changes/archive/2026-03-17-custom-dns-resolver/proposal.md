## Why

In GFW-censored environments, system DNS is often hijacked and returns `0.0.0.0` for blocked domains like `raw.githubusercontent.com`. This causes the GFW list fetcher to fail with "connection refused" and also breaks PAC DNS functions (`dnsResolve`, `isResolvable`) used for routing decisions. This change introduces a configurable custom DNS resolver that bypasses compromised system DNS by using trusted public DNS servers (e.g., Cloudflare 1.1.1.1, Google 8.8.8.8) for **both** PAC evaluation and GFW fetching.

## What Changes

- Add `custom_dns_servers` configuration to both `pac:` and `gfw:` sections in YAML
- Modify PAC DNS lookup to use custom resolver if configured
- Modify GFW fetcher to use custom resolver if configured
- Log which DNS resolver is in use at startup
- Maintain backward compatibility: empty list means use system DNS

## Capabilities

### New Capabilities
- `custom-dns-resolver`: Configurable DNS resolution that bypasses system DNS, used by both PAC evaluation and GFW fetching

### Modified Capabilities
(empty)

## Impact

- Modified files:
  - `pac/config.go` (add `CustomDNSServers` to `EvaluatorConfig`)
  - `pac/dns.go` (make `lookupHost` a method on `dnsCache` with optional custom resolver)
  - `pac/builtins.go` (call `cache.lookupHost` instead of package function)
  - `cmd/proxy-pac/main.go`:
    - Extend `Config.GFW` with `CustomDNSServers`
    - Update `loadConfig` to set defaults
    - After creating evaluator, set custom resolver if configured
    - After creating GFW fetcher, set custom HTTP client if configured
- New dependencies: none (pure stdlib)
- Configuration: optional `custom_dns_servers` list under `pac` and `gfw`
- Risk: If custom DNS servers are blocked, both PAC and GFW may fail; empty list disables custom DNS
