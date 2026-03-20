# Deepest Match Feature Specification

## Overview

The Deepest Match feature modifies the native rule engine's matching semantics to prefer the most specific (longest) suffix and most specific host‑glob pattern, rather than the rule with the lowest index. This is an opt‑in feature controlled by `pac.deepest_match` configuration (default `false`).

## Requirements

### Functional

1. **Exact matches** remain unchanged (exact host is always unique).
2. **Domain suffixes**: when multiple suffixes match the host, the engine must select the rule associated with the **longest suffix**. If multiple rules exist at the same longest suffix node, the one with the lowest index wins.
3. **Host globs**: When `deepest_match` is enabled, glob patterns are sorted by specificity (descending), with index as tie‑breaker. Evaluation iterates the sorted list and returns the first matching rule.
4. The feature is **disabled by default** to preserve backward compatibility.
5. The engine must continue to respect rule order **within the same specificity** (index as tie‑breaker).
6. URL‑glob rules continue to be ignored (no change).

### Non‑Functional

- Performance: Suffix traversal depth is limited (< 7 labels); hostGlob sorting occurs only on engine rebuild. No noticeable latency increase.
- Memory: Negligible extra fields (int per glob).
- Compatibility: Must not affect behavior when `deepest_match: false`.

## Detailed Design

### Configuration

```yaml
pac:
  deepest_match: false  # default
```

### Suffix Matching

The reversed‑label trie is unchanged. The `match` method is extended:

```go
func (t *domainSuffixTrie) match(host string, deepest bool) *ruleMeta {
    // ... normalize host, split and reverse labels ...
    bestIndex := maxInt
    bestDepth := -1
    var bestRule *ruleMeta
    node := t.root
    for depth, label := range labels {
        child, ok := node.children[label]
        if !ok { break }
        node = child
        if node.rule != nil {
            if deepest {
                if depth > bestDepth || (depth == bestDepth && node.rule.index < bestIndex) {
                    bestDepth = depth
                    bestIndex = node.rule.index
                    bestRule = node.rule
                }
            } else {
                if node.rule.index < bestIndex {
                    bestIndex = node.rule.index
                    bestRule = node.rule
                }
            }
        }
    }
    return bestRule
}
```

### HostGlob Specificity

**Specificity score** = `len(pattern) - (count('*') + count('?'))`. This favors longer patterns and penalizes wildcards.

**Sorting**:

```go
if e.deepest {
    for i := range e.hostGlobs {
        e.hostGlobs[i].spec = specificityScore(e.hostGlobs[i].pat)
    }
    sort.Slice(e.hostGlobs, func(i, j int) bool {
        if e.hostGlobs[i].spec != e.hostGlobs[j].spec {
            return e.hostGlobs[i].spec > e.hostGlobs[j].spec
        }
        return e.hostGlobs[i].meta.index < e.hostGlobs[j].meta.index
    })
}
```

During evaluation, the (now sorted) globs are iterated in order; the first match is returned.

### Data Structure Changes

```go
type ruleMetaRegex struct {
    meta  ruleMeta
    regex *regexp.Regexp
    pat   string   // original pattern string
    spec  int      // computed only if deepestMatch
}
```

## Examples

### Example 1: Suffix longest wins

```
Rules (indices):
0: .com          → SOCKS5 A
1: .google.com   → TUN
```

Host: `mail.google.com`

- `deepest_match: false` → rule 0 wins (lower index) → SOCKS5 A
- `deepest_match: true`  → rule 1 wins (longer suffix) → TUN

### Example 2: HostGlob specificity

```
Rules:
0: *.com        → SOCKS5 A
1: *.google.com → TUN
2: *            → BLOCK
```

- `deepest_match: false`: index order → rule 0 wins for `mail.google.com` (first match)
- `deepest_match: true`: specificity order → `*.google.com` (higher score) wins

## Testing

### Unit Tests

- `TestDeepestMatch_SuffixLongestWins`
- `TestDeepestMatch_HostGlobSpecificity`
- `TestDeepestMatch_Disabled` (ensures first‑match still works)

### Integration

- Extend `integration_test.go` or `pac/evaluator_native_test.go` with mixed tier tests.

## Migration Impact

- Default behavior unchanged.
- Existing users can enable `deepest_match` to get more intuitive matching without reordering rules.
- GFW list may not need manual reordering in some cases.

## Open Issues

- Should `deepest_match` also affect the Admin UI rule ordering display? Probably not; UI shows user‑defined order.
- Should we add a debug log line when a rule is selected by deepest match vs index? Not necessary for release, could be added in verbose mode.

## Future

- Consider making deepest_match the default in a future major version, after deprecation period.
- Explore specificity metrics for other match types (e.g., URL‑glob when supported).
