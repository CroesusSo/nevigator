# Tasks for Deepest Match Semantics

## Category 1: Configuration
- [ ] 1.1 Add `DeepestMatch bool` to `pac/config.go` with yaml/json tags.
- [ ] 1.2 Add `DeepestMatch` to `cmd/proxy-pac/main.go` Config struct.
- [ ] 1.3 Set default `DeepestMatch = false` in `loadConfig`.
- [ ] 1.4 Include `deepest_match: false` in `defaultConfigYAML`.
- [ ] 1.5 Add `DeepestMatch` to `admin/handlers.go` `FullConfig` (with yaml/json tags) and ensure it's read/written in APIs.

## Category 2: Engine Modifications
- [ ] 2.1 Extend `ruleMetaRegex` with `pat string` and `spec int` fields.
- [ ] 2.2 Modify `NewNativeRuleEngine` to accept `deepestMatch bool` and store in `NativeRuleEngine.deepest`.
- [ ] 2.3 If `deepestMatch`, compute `spec` for each hostGlob using `specificityScore` and sort the slice by (spec desc, index asc).
- [ ] 2.4 Update `domainSuffixTrie.match` to take `deepest bool` and implement deepest‑node logic when true; fall back to old logic when false.
- [ ] 2.5 Add `specificityScore` function in `native_engine.go`.
- [ ] 2.6 Modify `NativeRuleEngine.Evaluate` to pass `deepest` flag to `suffixTrie.match` and use pre‑sorted hostGlobs (first‑match still short‑circuits).

## Category 3: Tests
- [ ] 3.1 Add `TestDeepestMatch_SuffixLongestWins` in `pac/evaluator_native_test.go`:
  - Create PAC with two suffix rules: `.com` and `.google.com` (different routes)
  - Enable deepest_match via config
  - Verify `mail.google.com` picks `.google.com` route
- [ ] 3.2 Add `TestDeepestMatch_HostGlobSpecificity`:
  - Rules: `*.com`, `*.google.com`, `*`
  - With deepest_match, verify `mail.google.com` matches `*.google.com`
- [ ] 3.3 Add `TestDeepestMatch_Disabled`:
  - Same setup but `deepest_match: false`
  - Verify first‑match order (`.com` wins over `.google.com` if lower index)
- [ ] 3.4 Add `TestDeepestMatch_ExactUnaffected` (optional).
- [ ] 3.5 Update any existing tests that might be affected by default (none, since default false).

## Category 4: Documentation
- [ ] 4.1 Update `README.md` to document `deepest_match` configuration and its effect.
- [ ] 4.2 Update `MIGRATION.md` (if exists) or create note about new feature.
- [ ] 4.3 Update `AGENTS.md` with the new config field if needed.

## Category 5: Validation & Release
- [ ] 5.1 Run full test suite (`go test ./...`) and ensure all pass.
- [ ] 5.2 Build binary and verify config parsing.
- [ ] 5.3 (Optional) Add benchmark for suffix matching to ensure no regression.
- [ ] 5.4 Create PR with description, linking to this change.

## Notes
- Backward compatible: default false preserves existing behavior.
- No changes to GFW generator needed; deepest_match only affects native engine evaluation.
- The hostGlob sorting must maintain index as secondary key to preserve user ordering for equal specificity.
