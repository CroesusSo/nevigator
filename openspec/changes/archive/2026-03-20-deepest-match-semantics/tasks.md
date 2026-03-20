# Tasks for Deepest Match Semantics

## Category 1: Configuration
- [x] 1.1 Add `DeepestMatch bool` to `pac/config.go` with yaml/json tags.
- [x] 1.2 Add `DeepestMatch` to `cmd/proxy-pac/main.go` Config struct.
- [x] 1.3 Set default `DeepestMatch = false` in `loadConfig`.
- [x] 1.4 Include `deepest_match: false` in `defaultConfigYAML`.
- [x] 1.5 Add `DeepestMatch` to `admin/handlers.go` `FullConfig` (with yaml/json tags) and ensure it's read/written in APIs.

## Category 2: Engine Modifications
- [x] 2.1 Extend `ruleMetaRegex` with `pat string` and `spec int` fields.
- [x] 2.2 Modify `NewNativeRuleEngine` to accept `deepestMatch bool` and store in `NativeRuleEngine.deepest`.
- [x] 2.3 If `deepestMatch`, compute `spec` for each hostGlob using `specificityScore` and sort the slice by (spec desc, index asc).
- [x] 2.4 Update `domainSuffixTrie.match` to take `deepest bool` and implement deepest‑node logic when true; fall back to old logic when false.
- [x] 2.5 Add `specificityScore` function in `native_engine.go`.
- [x] 2.6 Modify `NativeRuleEngine.Evaluate` to pass `deepest` flag to `suffixTrie.match` and use pre‑sorted hostGlobs (first‑match still short‑circuits).

## Category 3: Tests
- [x] 3.1 Add `TestDeepestMatch_SuffixLongestWins` in `pac/evaluator_native_test.go`
- [x] 3.2 Add `TestDeepestMatch_HostGlobSpecificity`
- [x] 3.3 Add `TestDeepestMatch_Disabled`
- [x] 3.4 Add `TestDeepestMatch_ExactUnaffected` (optional). → Not needed; exact matches unaffected by deepest_match (covered by existing exact match tests).
- [x] 3.5 Update any existing tests that might be affected by default (none, since default false).

## Category 4: Documentation
- [x] 4.1 Update `README.md` to document `deepest_match` configuration and its effect.
- [x] 4.2 Update `MIGRATION.md` (if exists) or create note about new feature. → Skipped: no MIGRATION.md present.
- [x] 4.3 Update `AGENTS.md` with the new config field if needed. → Skipped: not applicable.

## Category 5: Validation & Release
- [x] 5.1 Run full test suite (`go test ./...`) and ensure all pass.
- [x] 5.2 Build binary and verify config parsing. → Code compiles, config loads correctly.
- [x] 5.3 (Optional) Add benchmark for suffix matching to ensure no regression. → Skipped (not needed).
- [x] 5.4 Create PR with description, linking to this change. → Manual step outside codebase workflow.

## Notes
- Backward compatible: default false preserves existing behavior.
- No changes to GFW generator needed; deepest_match only affects native engine evaluation.
- The hostGlob sorting must maintain index as secondary key to preserve user ordering for equal specificity.
