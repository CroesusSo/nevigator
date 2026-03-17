## Context

The proxy-pac application supports routing traffic through a TUN device when UseTUN is enabled. However, the GFW (Great Firewall) list fetcher was incorrectly binding its HTTP client to the TUN interface when fetching the gfwlist.txt file from raw.githubusercontent.com. This caused DNS resolution to fail because the TUN interface either doesn't have proper DNS configuration or returns 0.0.0.0 for external domain lookups.

The core issue is a separation of concerns:
- TUN interface: Used for routing proxied client traffic (outbound connections)
- System default interface: Used for application-level operations like fetching updates

## Goals / Non-Goals

**Goals:**
- Fix GFW list fetching when UseTUN is enabled
- Ensure the GFW fetcher always uses the system's default network interface
- Maintain backward compatibility when UseTUN is disabled
- Keep all existing GFW fetcher functionality (caching, ETag support, etc.)
- Add clear documentation/comments explaining the design decision

**Non-Goals:**
- Changing how TUN interface is used for routing proxied traffic
- Modifying the PAC evaluation or routing logic
- Altering the GFW list parsing or rule processing
- Changing the admin UI or API endpoints

## Decisions

### Prevent GFW Fetcher TUN Binding
**Decision:** Do not bind the GFW fetcher's HTTP client to the TUN interface, even when UseTUN is enabled.

**Rationale:**
- The GFW list must be fetched from public GitHub (raw.githubusercontent.com)
- This requires proper DNS resolution which may not work through the TUN interface
- The TUN interface is intended for client traffic routing, not application updates
- This follows the principle that infrastructure components should not interfere with application maintenance operations

**Alternatives Considered:**
1. Keep TUN binding but fix DNS resolution through TUN - Rejected because it would require configuring DNS within the TUN stack, adding complexity
2. Make TUN binding configurable - Rejected because it adds complexity with little benefit; the correct approach is consistent
3. Use a separate HTTP client just for GFW fetching - Rejected because it's redundant; just don't bind the existing client

### Add Informative Logging
**Decision:** Add a log message when TUN is configured to clarify that the GFW fetcher uses the system default interface.

**Rationale:**
- Helps users understand why GFW fetching works even when TUN is enabled for routing
- Makes the design decision visible in logs for troubleshooting
- Zero runtime cost when UseTUN is disabled

## Risks / Trade-offs

[Potential confusion about interface usage] → Mitigated by clear logging and documentation
[No functional impact when UseTUN is disabled] → No risk, maintains existing behavior
[Slightly different network path for GFW fetching] → Acceptable trade-off for reliability

## Migration Plan

1. Deploy the updated binary
2. No configuration changes required
3. GFW list fetching will work correctly when UseTUN is enabled
4. Rollback is simply reverting to previous binary version

## Open Questions

None