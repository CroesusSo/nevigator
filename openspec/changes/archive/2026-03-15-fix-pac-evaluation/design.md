## Context

`pac/evaluator.go:160` uses `fnVal.Export().(func(string, string) string)` to extract the JS function. goja's `Export()` for a JS-defined function returns `func(goja.FunctionCall) goja.Value`, so the type assertion always fails. This means PAC evaluation never actually calls `FindProxyForURL` — every request falls through to the fallback route (DIRECT).

## Goals / Non-Goals

**Goals:**
- Fix PAC function extraction so `FindProxyForURL` is actually called
- Add a testable routing rule to the default PAC (google.com → TUN)
- Update tests to verify both routing paths work

**Non-Goals:**
- Changing PAC builtins, DNS cache, or evaluation timeout
- Adding new PAC builtin functions
- Changing the fallback behavior when PAC evaluation fails

## Decisions

### Use `goja.AssertFunction()` instead of `Export()` type assertion

`goja.AssertFunction(fnVal)` returns `(func(goja.FunctionCall, ...goja.Value) (goja.Value, error), bool)`. This is the canonical way to extract callable JS functions from goja.

Calling convention:
```go
fn, ok := goja.AssertFunction(fnVal)
if !ok { /* not a function */ }
result, err := fn(goja.Undefined(), vm.ToValue(url), vm.ToValue(host))
```

`goja.Undefined()` is the `this` value. `vm.ToValue()` converts Go strings to goja values.

### Default PAC with testable routing rule

```js
function FindProxyForURL(url, host) {
  if (dnsDomainIs(host, ".google.com")) {
    return "TUN";
  }
  return "DIRECT";
}
```

Uses `dnsDomainIs` (already implemented in `builtins.go:26`) which is a standard PAC builtin. This covers subdomains (`.google.com` matches `www.google.com`, `mail.google.com`).

## Risks / Trade-offs

- **Tests depend on specific default PAC content**: If the default PAC changes again, `TestDefaultPACReturnsDirect` needs updating. Mitigation: the test validates behavior, not exact content.
- **TUN route may fail in test environments**: `TestEvaluator` using the new default PAC will try to route google.com through TUN. Mitigation: test verifies route *strategy* (TUN), not that the connection succeeds.
