# Native Engine IPv6 Fix – Proposal

## Problem

The native rule engine (used for high-performance PAC evaluation) incorrectly handles IPv6 addresses due to naive port stripping using `strings.IndexByte(host, ':')`. This breaks IPv6 destination matching for:

- **Exact match rules** (`MatchExact`): An IPv6 literal like `2001:db8::1` gets truncated to `"2001"` during rule building and evaluation, making matching impossible.
- **Host glob patterns** (`MatchHostGlob`): Same truncation issue prevents pattern matches against IPv6 hosts.
- **Domain suffix trie**: Contains similar defensive truncation (harmless for IPv6 but inconsistent).

**Example failure:**
- Rule: `{MatchExact, Value: "2001:db8::1", Route: TUN}`
- Evaluation with host `"2001:db8::1"`: host truncated to `""` → no match → falls back incorrectly.

Additionally, the `hostGlobs` evaluation loop uses `break` on first match, preventing proper rule ordering (lowest index wins). This violates the intended priority semantics where rule order matters.

## Proposed Solution

1. **Replace colon truncation** with proper `net.SplitHostPort` in three locations:
   - `Build()` for `MatchExact` (rule value cleaning)
   - `Evaluate()` (incoming host cleaning)
   - `domainSuffixTrie.match()` (consistency)

2. **Fix hostGlobs priority** by removing `break` and continuing to scan all matching globs to find the one with the lowest `index`.

3. **Add comprehensive IPv6 tests** for native engine:
   - Exact match with IPv6 literal
   - Host glob match with IPv6 pattern (e.g., `2001:db8::*`)
   - Ensure domain suffix patterns do not match IPv6 (as expected)

## Scope

- **File:** `pac/native_engine.go`
  - Modify `Build()`: `case gfw.MatchExact`
  - Modify `Evaluate()`
  - Modify `domainSuffixTrie.match()`
  - Modify hostGlobs loop (remove break)
- **File:** `pac/native_engine_test.go`
  - Add IPv6 test cases
- **No changes** to other packages, PAC parsing, or generated PAC format.

## Benefits

- Full IPv6 support in native rule engine (same parity as JS engine)
- Correct rule ordering for host globs
- Cleaner, more maintainable code (defensive port handling)

## Risks & Mitigations

- **Risk:** Performance regression from `net.SplitHostPort` on every evaluation.
  - **Mitigation:** Host is typically clean (no port), so error path is cheap; still measurable? Benchmark needed.
- **Risk:** Breaking existing behavior for hosts with accidental ports.
  - **Mitigation:** Current truncation already strips ports; new approach strips more correctly. Existing tests should pass.

## Out of Scope

- `MatchURLGlob` remains unsupported (limitation of CONNECT proxy, not addressable here).
- Performance optimizations beyond correctness.
