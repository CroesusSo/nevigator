# Deepest Match Semantics for Native Rule Engine

## Problem

Current native rule engine uses **first-match** semantics within each rule type (suffix, hostGlob). For domain suffixes, this means the rule with the **lowest index** wins when multiple suffixes match, regardless of suffix length.

### Example Issue

```
Rule order (by index):
1. Match: domain_suffix, Value: ".com"         → Route: SOCKS5 proxy-A
2. Match: domain_suffix, Value: ".google.com" → Route: TUN

For host "mail.google.com":
- Both suffixes match
- Rule 1 (".com") has lower index → wins
- Result: traffic goes to proxy-A instead of TUN ❌
```

This is counterintuitive because users expect more specific suffixes to override generic ones, without having to manually reorder rules.

JavaScript PAC (dnsDomainIs) uses first-match as well, but the typical GFWlist and user rule patterns rely on correct ordering; the problem surfaces when generic rules appear before specific ones. Our reordering change (exceptions before blacklist) helps but doesn't address suffix length ordering within each category.

## Solution

Introduce an **optional** `deepest_match: true` configuration (default `false` for backward compatibility). When enabled:

- **Domain suffixes**: pick the rule from the **longest matching suffix** (deepest node in reversed-label trie). Ties broken by lowest index.
- **Host globs**: pick the rule with the **most specific pattern** (highest specificity score, then lowest index). Specificity = pattern length penalty for wildcards (`*`, `?`).
- Exact matches remain unchanged (exact is exact).

This makes matching behavior more intuitive: *specificity beats generality*, regardless of rule order.

## Scope

### In Scope
- Add `deepest_match bool` to `pac.Config` and full config structs (YAML/JSON).
- Modify `domainSuffixTrie.match` to return rule from deepest matching node.
- Modify hostGlob handling: compute specificity during engine build, sort globs by (specificity descending, index ascending), and keep short‑circuit evaluation.
- Update `NativeRuleEngine.Evaluate` to use new suffix match and the pre‑sorted hostGlobs.
- Add unit tests:
  - `TestDeepestMatch_SuffixLongestWins`
  - `TestDeepestMatch_HostGlobSpecificity`
  - `TestDeepestMatch_DisabledByDefault`
- Update integration tests to cover deepest_match scenarios.
- Update documentation (README, MIGRATION.md).
- Preserve backward compatibility: default `false` retains first‑match.

### Out of Scope
- Changing URL‑glob behavior (still not supported).
- Changing exact match behavior.
- Modifying rule precedence between categories (exact → suffix → glob order remains).
- JS PAC evaluator (already removed).

## Dependencies
- Builds on the native‑only engine (remove‑js‑evaluator change must be merged first).
- No new external dependencies.

## Risks & Mitigations
| Risk | Mitigation |
|------|------------|
| Behavior change breaks existing PACs that rely on first‑match | Config flag defaults to `false`; users opt‑in. Document differences clearly. |
| Deeper suffix traversal slightly slower | Suffix depth is limited (max ~5‑6 labels); impact negligible. |
| HostGlob specificity scoring might be arbitrary | Use simple metric: pattern length penalized by number of `*`/`?`. Document and tune if needed. |
| Sorting globs loses index order for tie‑break | Keep index as secondary sort key. |

## Alternatives Considered
- **Sort suffixes by length during PAC generation** – would require reordering user/GFW rules, altering displayed order in UI; less clean than runtime semantics.
- **Always use deepest match without opt‑out** – breaking change; opt‑in safer.
- **Deepest only for suffixes, leave globs first‑match** – inconsistent; better to provide most‑specific for both.

## Open Questions
- Should `deepest_match` also affect how we display/preview matching rules in the admin UI? Probably not needed; UI shows rule order as stored.
- Should we add a log message when a suffix match falls back to a deeper rule vs first‑match? Optional debug only.

## Success Criteria
- All existing tests pass with `deepest_match: false` (default).
- New tests demonstrate correct deepest‑match behavior when enabled.
- Documentation clearly explains the feature and when to use it.
- No performance regression for default path (first‑match still fast).
