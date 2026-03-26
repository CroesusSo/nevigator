## Context

The proxy currently supports three routing strategies: TUN (bind to TUN device IP), SOCKS5 (upstream proxy), and DIRECT (system default). TUN support requires:
- Interface discovery (net.InterfaceByName)
- IP extraction from interface addresses
- Status monitoring (is interface up?)
- Configuration field `tun_device`
- UI controls for TUN device name
- Status reporting (tunDevice, tunIP, tunUp)
- Specialized error handling (ErrTUNNotConfigured)

The TUN route strategy is implemented by binding the outbound dialer's LocalAddr to the TUN interface IP. This forces traffic through a VPN tunnel.

Approximately 20 files contain TUN-specific code. The test suite includes dedicated TUN tests that will need removal or rewriting.

## Goals / Non-Goals

**Goals:**
- Remove all TUN device routing functionality from the codebase
- Simplify configuration by eliminating `tun_device` and related fields
- Simplify admin UI by removing TUN device settings and TUN route option
- Reject any configuration or PAC files that reference TUN with clear error messages
- Maintain existing SOCKS5, EXTERNAL, and DEFAULT route functionality unchanged
- Preserve automatic fallback to DIRECT on route failure (already independent of TUN)

**Non-Goals:**
- Provide automatic migration from TUN to SOCKS5 routes (users must manually update PAC rules)
- Support any form of interface binding (users requiring VPN should use SOCKS5 over the VPN)
- Backward compatibility with existing TUN configurations (this is a breaking change)
- Modify the external-proxy-registry capability (it remains unchanged)

## Decisions

### 1. Validation Strategy: Strict Rejection

**Decision:** The system will fail startup if the configuration contains any TUN-related field (`tun_device` in proxy config, `use_tun` in gfw config). The PAC loader will reject PAC files containing any rule with `"route":"TUN"`.

**Rationale:** 
- Silent fallback to DIRECT would be surprising and could break intended routing
- Strict validation forces users to consciously update their configs and PAC rules
- Error messages can guide users toward SOCKS5 or EXTERNAL alternatives
- Avoids ambiguity about deprecated field handling

**Alternatives Considered:**
- *Lenient:* Silently treat TUN routes as DIRECT. Rejected due to silent behavior change.
- *Migration:* Auto-convert TUN → DIRECT. Rejected because users likely have specific routing intent; auto-conversion could cause traffic leaks.

### 2. Config Handling: Error on Load

**Decision:** When loading the YAML config, if `proxy.tun_device` or `gfw.use_tun` is present, return an error during `loadConfig()` and exit with a message: "TUN device support has been removed. Remove 'tun_device' from proxy config and 'use_tun' from gfw config."

**Rationale:**
- Fail fast at startup rather than later in runtime
- Clear, actionable error message
- No need to persist modified configs (user must edit manually)

**Alternatives Considered:**
- *Auto-strip:* Remove TUN fields from config automatically. Rejected because it silently modifies user configuration, potentially losing information.

### 3. PAC Loading: Reject TUN Routes

**Decision:** In `NewNativeRuleEngine()`, when parsing `__PAC_RULES__`, if any rule has `route` field equal to `"TUN"`, return an error: "TUN routing is no longer supported. Use SOCKS5 or EXTERNAL proxy instead."

**Rationale:**
- PAC rules explicitly declaring TUN indicate user intent that must be addressed
- Prevents runtime routing to non-existent TUN capability
- Forces manual review and update of PAC files

**Alternatives Considered:**
- *Rewrite:* Auto-change TUN → DIRECT. Too dangerous; could cause unintended direct connections.
- *Skip:* Silently ignore TUN rules. Would silently drop routing logic, potentially breaking expected behavior.

### 4. Status API: Remove TUN Fields

**Decision:** Remove `TUNDevice`, `TUNIP`, `TUNUp` from `ServerStatus` struct. Admin UI will no longer display TUN status.

**Rationale:**
- Clean removal; no stub fields or "not configured" placeholders needed
- Simplifies status tracking and UI

### 5. Test Suite: Remove TUN Tests

**Decision:** Delete all test functions that specifically test TUN routing:
- `TestRouterFallback` (rewrite to test SOCKS5 fallback instead)
- Tests in `pac/evaluator_test.go`, `pac/evaluator_native_test.go`, `pac/native_engine_test.go`, `pac/builtins_test.go`, `pac/bench_test.go` that use TUN routes
- Integration tests referencing TUN

**Rationale:**
- Test suite should validate supported functionality only
- TUN-specific tests will fail after removal and must be cleaned up
- Replacement tests for fallback behavior ensure coverage isn't lost

**Alternatives Considered:**
- *Comment out tests:* Leaves dead code; not appropriate for complete removal.

## Risks / Trade-offs

**Risk:** Existing users relying on TUN will be unable to start the proxy after upgrade.  
*Mitigation:* Clear release notes, migration guide, and error messages explaining how to switch to SOCKS5/EXTERNAL proxies. This is an expected breaking change.

**Risk:** Some users may have used TUN for privacy/VPN purposes and not understand alternatives.  
*Mitigation:* Documentation should emphasize that SOCKS5 routes can be used over a VPN by configuring a SOCKS5 proxy running on the VPN gateway.

**Trade-off:** We lose the ability to force traffic through a specific interface without running a separate proxy. This is intentional; users who need that should run a local SOCKS5 proxy bound to the TUN interface.

**Risk:** Incomplete removal—TUN references left in code could cause runtime panics (e.g., nil pointer when accessing tunIP).  
*Mitigation:* Careful code review, grep for remaining `tun` references, test compilation with TUN-using configs to ensure errors are caught early.

## Migration Plan

1. **Release preparation:**
   - Update config.yaml.example to remove `tun_device` line
   - Update README.md to remove TUN documentation and examples
   - Add migration guide: "Replacing TUN Routes" → use SOCKS5 or EXTERNAL proxy

2. **Upgrade experience:**
   - User upgrades binary, starts with existing config containing `tun_device`
   - Startup fails with clear error: "TUN device support has been removed. Remove 'tun_device' from proxy config."
   - User removes `tun_device` and updates PAC rules to use SOCKS5/EXTERNAL/DIRECT
   - User restarts successfully

3. **Rollback:** Users must revert to previous version; no automated rollback.

## Open Questions

1. **Should we keep the `Route.Strategy` string constant "TUN" in code for documentation purposes?**  
   Proposed: Delete entirely; no sentiment needed for removed feature.

2. **What to do with the `deepest_match` feature's relation to TUN?**  
   No impact; `deepest_match` applies to all route types. Keep unchanged.

3. **Should the Admin UI still show "DIRECT" and "DEFAULT" only, or add SOCKS5/DEFAULT/EXTERNAL explicitly?**  
   Proposed: Keep route selector as: DIRECT, SOCKS5 (with address field), EXTERNAL (dropdown of proxies), DEFAULT.

4. **Update the capability spec `pac-proxy-tun.md`:** Should it be renamed to `pac-routing.md` or deleted?  
   Proposed: Rename/modify to reflect that routing is supported but TUN is not. Actually, the spec covers more than TUN—it covers routing strategies. Should be renamed to `pac-routing` and updated to remove TUN references.
