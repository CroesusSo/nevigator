# Native Rule Engine and LRU Caching – Proposal

## Problem
The current PAC evaluator uses a JavaScript runtime (goja) for every request, creating a fresh VM and executing the PAC script. With the GFW list containing 4100+ rules (plus any user rules), the generated PAC becomes a long chain of ~4100 JavaScript `if` statements. This results in:

- High per-request latency (1–5ms) due to VM creation and interpretation.
- Poor scalability under concurrent load.
- Unnecessary CPU overhead for a problem that can be solved with simple data structures.

A significant portion of this work is repeated for each request, and the same hosts are evaluated many times (e.g., popular sites like Google, Facebook). There is currently no caching.

## Proposed Solution
Replace the JavaScript interpreter for **generated PAC files** (those containing a `__PAC_RULES__` variable) with a **native Go rule engine** that:

1. Parses the embedded JSON rules directly.
2. Builds efficient indexes:
   - **Exact map** for exact host matches (O(1)).
   - **Reversed-label trie** for domain suffixes (O(depth) ≈ 4).
   - **Ordered, deduplicated slice** for host glob patterns (early exit on first match).
3. Evaluates routes by choosing the rule with the smallest index (first in PAC) among all matching candidates.
4. Adds an **LRU cache with TTL** (10 minutes, max 10,000 entries) to store recent host→route results.
5. Invalidates cache automatically when the PAC file changes (including GFW list updates).

For custom PAC scripts without `__PAC_RULES__`, the system will continue to use the JS evaluator unchanged.

## Benefits
- **>100× performance improvement** for uncached evaluation (sub‑microsecond vs millisecond).
- **Cache hit latency** < 0.1 µs.
- Reduced CPU load, enabling many more concurrent connections.
- Maintains perfect PAC semantics: first match wins, case‑insensitive, same fallback behavior.
- No changes to configuration or user‑facing behavior; transparent upgrade.

## Scope
- Implement native rule engine (`native_engine.go`) and unit tests.
- Implement LRU cache (`cache.go`) and unit tests.
- Modify `Evaluator` to detect native‑capable PAC and switch automatically.
- Add cache invalidation on reload.
- Add benchmarks to compare JS vs native with realistic rule counts.
- Add optional admin API endpoint to show cache statistics.

## Out of Scope
- Supporting URL‑glob rules (`shExpMatch(url, ...)`) because the proxy only knows the host, not the full URL for CONNECT. These rules remain non‑functional in both JS and native modes.
- Changing the PAC file format.
- Altering the rule generation process (`gfw.GeneratePAC`).

## Risks and Mitigations
- **Risk:** Malformed `__PAC_RULES__` JSON could crash the proxy.  
  **Mitigation:** Catch errors during native engine construction and fall back to JS compilation; log warning.
- **Risk:** Native engine might diverge from JS semantics for edge cases.  
  **Mitigation:** Comprehensive test suite covering mixed rule types, duplicates, ordering, and case handling; compare outputs with JS for many PAC samples.
- **Risk:** Cache size/TTL too small/large.  
  **Mitigation:** Make parameters configurable (defaults: 10k, 10min). Provide admin metrics to tune.
