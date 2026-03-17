## Context

The current GFW rule processing flow:
1. GFW fetcher downloads/parses gfwlist.txt into Rule structs
2. PAC generator converts rules to PAC JavaScript
3. PAC evaluator runs the script and returns proxy strings
4. PAC parser converts strings to Route structs
5. Router handles Route structs to establish connections

Currently:
- GFW blacklist rules (no @@) get Route = "SOCKS5 127.0.0.1:1080" (hardcoded)
- GFW whitelist rules (@@) get Route = "DIRECT"
- When no proxy runs at 127.0.0.1:1080, router fails and falls back to DIRECT

## Goals / Non-Goals

**Goals:**
- GFW blacklist rules should use configured default external proxy when available
- When no default proxy configured, GFW blacklist routes should go DIRECT
- Eliminate unnecessary connection attempts to hardcoded proxy addresses
- Maintain backward compatibility for whitelist rules
- Keep router unchanged to leverage existing fallback logic

**Non-Goals:**
- Modify router fallback behavior (to be considered separately)
- Change how user-defined PAC rules are handled
- Alter GFW rule storage or fetching mechanisms
- Modify whitelist rule behavior (remains DIRECT)

## Decisions

### Parser Modification: Return Empty Route for Blacklist
**Choice:** Modify `gfw/parser.go` route() function to return empty string for blacklist entries
**Alternative:** Keep hardcoded route and modify generator to override it
**Rationale:** 
- Clean separation: parser reflects raw rule meaning, generator applies policy
- Empty route signals "apply DEFAULT/DIRECT logic" clearly
- Minimal change to existing parsing logic
- Allows generator to make contextual decisions based on configuration

### Generator Logic: DEFAULT when configured, DIRECT when not
**Choice:** In `gfw/generator.go`, when route is empty:
- If DefaultExternalProxy configured → return "DEFAULT" 
- Else → return "DIRECT" (changed from hardcoded "SOCKS5 127.0.0.1:1080")
**Alternative:** Return special "BLOCK" value requiring router changes
**Rationale:**
- Achieves desired behavior without router changes
- "DIRECT" fallback is safe and leverages existing router logic
- More semantically correct: no proxy = direct connection
- Avoids connection attempts to non-existent hardcoded addresses
- Maintains semantic equivalence to current behavior (end result still DIRECT when no proxy)

### Whitelist Rules Unchanged
**Choice:** Keep whitelist rules returning "DIRECT" in both parser and generator
**Alternative:** Apply same DEFAULT/DIRECT logic to whitelist rules
**Rationale:**
- Whitelist meaning is "always direct" regardless of proxy configuration
- Changing would alter fundamental semantics of whitelisting
- No indication this behavior needs to change
- Keeps change focused and minimal

## Risks / Trade-offs

[Risk] PAC evaluation returns "DEFAULT" when no default proxy configured → router error → fallback to DIRECT
[Mitigation]: This is acceptable intermediate state - the end result is still DIRECT, but with logging that indicates misconfiguration. Users see in logs: "Route DEFAULT failed: no default external proxy configured; falling back to DIRECT"

[Risk] Breaking change for users who rely on hardcoded proxy behavior
[Mitigation]: Those users were already experiencing failed connections and fallbacks. New behavior is more efficient (no connection attempt) and equally safe. Users wanting explicit proxy can configure default external proxy.

[Risk] Inconsistent handling between user rules and GFW rules
[Mitigation]: This change only affects GFW-sourced rules. User rules continue to work as before. Consistency can be addressed in future work if needed.

## Migration Plan

1. Deploy updated GFW parser and generator
2. Existing configurations with default external proxy: 
   - Before: GFW blacklist → hardcoded proxy → [if works] use it, [if fails] fallback to DIRECT
   - After: GFW blacklist → DEFAULT → uses configured proxy
   - Improvement: Eliminates failed connection attempts when proxy unavailable
3. Existing configurations without default external proxy:
   - Before: GFW blacklist → hardcoded proxy → [always fails] → fallback to DIRECT (logged)
   - After: GFW blacklist → DIRECT → direct connection
   - Improvement: Eliminates unnecessary connection attempts and error logs
4. Whitelist rules unchanged in both cases

## Open Questions

None identified - the approach is straightforward and leverages existing safe fallback mechanisms.