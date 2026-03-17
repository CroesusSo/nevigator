## Why

When the UseTUN option is enabled in the proxy-pac configuration, the GFW fetcher incorrectly binds its HTTP client to the TUN interface for fetching the gfwlist.txt file. This causes DNS resolution failures when trying to resolve raw.githubusercontent.com, resulting in connection errors to 0.0.0.0:443. The GFW list must be fetched using the system's default network interface to ensure proper DNS resolution, independent of the TUN interface used for routing proxied traffic.

## What Changes

- Modify cmd/proxy-pac/main.go to prevent the GFW fetcher from binding to the TUN interface
- Add clear comments explaining why the GFW fetcher must use the system default interface
- Maintain all other GFW fetcher functionality (fetching, caching, PAC regeneration)
- Add informational logging when TUN is configured to clarify the separation of concerns

## Capabilities

### New Capabilities
None

### Modified Capabilities
- `gfw-fetcher`: Changes the requirement that the GFW fetcher must use the system's default network interface for HTTP requests, regardless of TUN configuration for traffic routing

## Impact

- Modified file: cmd/proxy-pac/main.go (GFW fetcher initialization section)
- No API changes
- No dependency changes
- Fixes GFW list fetching when UseTUN is enabled
- Maintains existing behavior when UseTUN is disabled