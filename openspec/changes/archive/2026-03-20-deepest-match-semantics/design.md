# Deepest Match Semantics – Technical Design

## 1. Configuration

Add field to `pac/config.go`:

```go
type Config struct {
    // ... existing fields ...
    EnableDeepestMatch bool `yaml:"enable_deepest_match" json:"enable_deepest_match"` // default false
}
```

Propagate through:
- `cmd/proxy-pac/main.go` (default in `loadConfig`)
- `admin/handlers.go` (FullConfig marshaling)
- `defaultConfigYAML` (include `enable_deepest_match: false`)
- `pac/EvaluatorConfig` – include a corresponding field so the evaluator can pass it to the engine.

The admin UI (admin/handlers.go HTML and client JavaScript) will expose the setting via a checkbox in the Settings tab, following the same pattern as `enable_url_glob`.

## 2. Engine Construction

Update `NewNativeRuleEngine` to accept `enableDeepest bool`:

```go
func NewNativeRuleEngine(pacContent string, fallback Route, enableDeepest bool) (*NativeRuleEngine, error)
```

Store the flag in the engine struct:

```go
type NativeRuleEngine struct {
    exactMap       map[string]*ruleMeta
    suffixTrie     *domainSuffixTrie
    hostGlobs      []ruleMetaRegex  // sorted by (specificity desc, index asc) if enabled
    fallback       Route
    enableDeepest  bool              // controls deepest/specificity matching
}
```

### 2.1 Call Site Integration

In `pac/evaluator.go`, modify the reload logic to pass the flag:

```go
engine, err := NewNativeRuleEngine(string(src), e.cfg.FallbackRoute, e.cfg.EnableDeepestMatch)
```

During engine build, when constructing host glob entries:

```go
case gfw.MatchHostGlob:
    pattern := r.Value
    if globSeen[pattern] {
        continue
    }
    regexStr := shellPatternToRegex(pattern)
    regex, err := regexp.Compile(regexStr)
    if err != nil {
        log.Printf("[WARN] Invalid host glob pattern %q: %v", pattern, err)
        continue
    }
    engine.hostGlobs = append(engine.hostGlobs, ruleMetaRegex{
        meta:  meta,
        regex: regex,
        pat:   pattern,   // store original pattern for specificity
    })
    globSeen[pattern] = true
```

After all rules are processed, if `enableDeepest` is true, compute specificity and sort:

```go
if engine.enableDeepest {
    for i := range engine.hostGlobs {
        engine.hostGlobs[i].spec = specificityScore(engine.hostGlobs[i].pat)
    }
    sort.Slice(engine.hostGlobs, func(i, j int) bool {
        if engine.hostGlobs[i].spec != engine.hostGlobs[j].spec {
            return engine.hostGlobs[i].spec > engine.hostGlobs[j].spec
        }
        return engine.hostGlobs[i].meta.index < engine.hostGlobs[j].meta.index
    })
}
```

## 3. Suffix Matching – Deepest Node

Modify `domainSuffixTrie.match`:

Current returns rule with lowest index among all visited nodes.

New behavior (pseudocode):

```go
func (t *domainSuffixTrie) match(host string, deepest bool) *ruleMeta {
    // ... normalize host, split labels reversed ...
    bestIndex := maxInt
    var bestRule *ruleMeta
    bestDepth := -1
    node := t.root
    for depth, label := range labels {
        child, ok := node.children[label]
        if !ok { break }
        node = child
        if node.rule != nil {
            // pick rule with deeper depth, or same depth with lower index
            if depth > bestDepth || (depth == bestDepth && node.rule.index < bestIndex) {
                bestDepth = depth
                bestIndex = node.rule.index
                bestRule = node.rule
            }
        }
    }
    return bestRule
}
```

If `deepest == false`, fall back to old behavior (lowest index only).

## 4. HostGlob Specificity Score

Define:

```go
func specificityScore(pattern string) int {
    // Longer patterns more specific; wildcards reduce score
    length := len(pattern)
    wildcards := strings.Count(pattern, "*") + strings.Count(pattern, "?")
    // Each wildcard subtracts a penalty; choose 1 per wildcard for simplicity
    // Could also weight '*' as 2, '?' as 1 if desired.
    return length - wildcards
}
```

Example:
- `*.google.com` (13 - 1 = 12)
- `*.com` (5 - 1 = 4)
- `*` (1 - 1 = 0)
- `www.example.com` (15 - 0 = 15) [though exact would be handled by exact map]

This naturally orders longer/less‑wild patterns higher.

## 5. Evaluate Integration

In `NativeRuleEngine.Evaluate`, the engine consults the suffix trie with the `enableDeepest` flag:

```go
func (e *NativeRuleEngine) Evaluate(host string) Route {
    // exact unchanged
    if m, ok := e.exactMap[host]; ok {
        return m.route
    }
    // suffix: match() returns the best rule according to deepest flag
    if m := e.suffixTrie.match(host, e.enableDeepest); m != nil {
        return m.route
    }
    // host globs: if deepest enabled, slice is pre‑sorted by specificity → first match wins
    for _, hg := range e.hostGlobs {
        if hg.regex.MatchString(host) {
            return hg.meta.route
        }
    }
    return e.fallback
}
```

Tier order (exact → suffix → glob → fallback) remains unchanged; each tier picks its best internally, but lower tiers never override a higher-tier match.

When `EnableDeepestMatch` is false, `suffixTrie.match` falls back to the original lowest‑index logic and the hostGlobs slice remains in original index order (no sorting).

## 6. Suffix Trie – Match Implementation

```go
func (t *domainSuffixTrie) match(host string, deepest bool) *ruleMeta {
    host = strings.ToLower(host)
    if h, _, err := net.SplitHostPort(host); err == nil {
        host = h
    }
    labels := strings.Split(host, ".")
    // reverse labels to trie order (TLD first)
    for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
        labels[i], labels[j] = labels[j], labels[i]
    }
    bestIndex := int(^uint(0) >> 1)
    bestDepth := -1
    var bestRule *ruleMeta
    node := t.root
    for depth, label := range labels {
        child, ok := node.children[label]
        if !ok {
            break
        }
        node = child
        if node.rule != nil {
            if deepest {
                // Prefer deeper nodes; if same depth, lower index wins
                if depth > bestDepth || (depth == bestDepth && node.rule.index < bestIndex) {
                    bestDepth = depth
                    bestIndex = node.rule.index
                    bestRule = node.rule
                }
            } else {
                // First‑match by index only
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

## 6. Tests

### Negative (first‑match) tests already exist:
- `TestEvaluate_ExceptionOverridesBlacklist` (index priority)
- Suffix and glob tests

### New tests:
- `TestDeepestMatch_SuffixLongestWins`: with `.com` and `.google.com` both matching, deepest picks `.google.com`.
- `TestDeepestMatch_HostGlobSpecificity`: patterns `*.com` vs `*.google.com` vs `*` – ensure correct ordering.
- `TestDeepestMatch_Disabled`: verify that setting `deepest_match: false` still uses index ordering.
- Benchmark: ensure no regression for default path.

## 7. Documentation

- Update README: describe `deepest_match` under PAC or Configuration.
- Note in MIGRATION.md that default is false, how to enable.
- Mention in AGENTS.md if needed for future agent builds.

## 8. Migration Path

No migration needed. Feature is opt‑in. Users who want more intuitive matching can enable it. Those relying on first‑match ordering keep existing behavior.

## 9. Open Questions

- Should we expose specificity algorithm details? Keep it simple.
- Should we consider a hybrid: deepest for suffixes, but not for globs? The config toggle covers both together; if users want mixed, they'd need separate flags, but that adds complexity.
