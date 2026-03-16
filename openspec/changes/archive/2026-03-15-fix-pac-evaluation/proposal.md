## Why

PAC evaluation is completely broken — `FindProxyForURL` is never called because `goja.Export()` returns a `func(goja.FunctionCall) goja.Value`, but the code asserts it as `func(string, string) string`. Every route falls back to DIRECT regardless of PAC rules. The default PAC script also returns "DIRECT" for everything, making it impossible to verify routing behavior during testing.

## What Changes

- **Fix `FindProxyForURL` extraction** in `pac/evaluator.go:Evaluate()` — use `goja.AssertFunction()` instead of broken type assertion
- **Update default PAC script** to route `google.com` through TUN, everything else DIRECT — enables testing routing behavior
- **Update tests** to match new default PAC behavior

## Capabilities

### New Capabilities
- `pac-evaluation`: PAC script evaluation with `FindProxyForURL` function calling and default routing rules

### Modified Capabilities

## Impact

- **Affected code**: `pac/evaluator.go` (Evaluate method), `pac/evaluator.go` (defaultPACContent constant)
- **Affected tests**: `pac/evaluator_test.go` — TestEvaluator, TestDefaultPACReturnsDirect
- **No changes to**: builtins, DNS, proxy server, router, admin UI, config
