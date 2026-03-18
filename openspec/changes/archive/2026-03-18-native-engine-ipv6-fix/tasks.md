# Native Engine IPv6 Fix – Tasks

## Implementation
- [x] Add `"net"` import to `pac/native_engine.go`
- [x] Fix `Build()` case `MatchExact`: replace colon truncation with `net.SplitHostPort`
- [x] Fix `Evaluate()`: replace colon truncation with `net.SplitHostPort`
- [x] Fix `domainSuffixTrie.match()`: replace colon truncation with `net.SplitHostPort` (consistency)
- [x] Fix hostGlobs loop: remove `break` to allow best-match by index
- [x] Verify no other colon-truncation patterns remain in `native_engine.go`

## Testing
- [x] Add `TestExact_IPv6`: exact match rule for IPv6 literal
- [x] Add `TestHostGlob_IPv6`: glob pattern like `2001:db8::*` matching correctly
- [x] Add `TestSuffix_IPv6_NoMatch`: domain suffix does not match IPv6 (sanity)
- [x] Add `TestHostGlob_Priority`: two overlapping globs, verify lower index wins
- [x] Run full test suite (`go test ./...`) – ensure no regressions

## Documentation & Review
- [x] Update README if needed (IPv6 native engine support already mentioned? Check)
- [x] Code review
- [x] Archive change after merge
